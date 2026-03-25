package net

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	"dmsg/internal/abuse"
	"dmsg/internal/filter"
	dmsgmsg "dmsg/internal/msg"
	"dmsg/internal/plugin"
	"dmsg/internal/trust"
)

const (
	TopicName      = "dmsg-global"
	MaxDedupCache  = 100_000
	DefaultDiff    = 8
	RatePerMinute  = 10
	RateBurst      = 20
	MaxMsgAge      = 24 * time.Hour
)

// Handler is called when a verified, filtered message arrives.
type Handler func(m *dmsgmsg.Message)

// Node wraps a libp2p host + GossipSub + trust + filter + abuse + plugins.
type Node struct {
	host      host.Host
	ps        *pubsub.PubSub
	topic     *pubsub.Topic
	sub       *pubsub.Subscription
	dedup     *dmsgmsg.DedupCache
	limiter   *dmsgmsg.RateLimiter
	replay    *dmsgmsg.ReplayGuard
	trust     *trust.Engine
	filters   *filter.Engine
	abuse     *abuse.Detector
	plugins   *plugin.Manager
	discovery *Discovery
	handler   Handler
	diff      int
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

// Config holds node configuration.
type Config struct {
	ListenAddr string
	Bootstrap  []string
	Rendezvous string
	Handler    Handler
	Difficulty int

	// Optional integrations
	PluginManager *plugin.Manager
}

// NewNode creates and starts a fully integrated dmsg P2P node.
func NewNode(cfg Config) (*Node, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// --- 1. libp2p host ---
	h, err := libp2p.New(
		libp2p.ListenAddrStrings(cfg.ListenAddr),
		libp2p.NATPortMap(),
		libp2p.EnableRelay(),
	)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("libp2p new: %w", err)
	}

	// --- 2. GossipSub ---
	ps, err := pubsub.NewGossipSub(ctx, h,
		pubsub.WithPeerScore(&pubsub.PeerScoreParams{
			AppSpecificScore: func(p peer.ID) float64 { return 0 },
			Topics:           map[string]*pubsub.TopicScoreParams{},
		}, &pubsub.PeerScoreThresholds{
			GossipThreshold:   -100,
			PublishThreshold:  -500,
			GraylistThreshold: -1000,
		}),
	)
	if err != nil {
		h.Close()
		cancel()
		return nil, fmt.Errorf("gossipsub: %w", err)
	}

	// --- 3. Topic ---
	topic, err := ps.Join(TopicName)
	if err != nil {
		h.Close()
		cancel()
		return nil, fmt.Errorf("join topic: %w", err)
	}
	sub, err := topic.Subscribe()
	if err != nil {
		topic.Close()
		h.Close()
		cancel()
		return nil, fmt.Errorf("subscribe: %w", err)
	}

	diff := cfg.Difficulty
	if diff == 0 {
		diff = DefaultDiff
	}

	// --- 4. Build node ---
	n := &Node{
		host:    h,
		ps:      ps,
		topic:   topic,
		sub:     sub,
		dedup:   dmsgmsg.NewDedupCache(MaxDedupCache),
		limiter: dmsgmsg.NewRateLimiter(float64(RatePerMinute)/60.0, RateBurst),
		replay:  dmsgmsg.NewReplayGuard(MaxMsgAge, MaxDedupCache),
		trust:   trust.NewEngine(),
		filters: filter.NewEngine(),
		abuse:   abuse.NewDetector(),
		plugins: cfg.PluginManager,
		handler: cfg.Handler,
		diff:    diff,
		ctx:     ctx,
		cancel:  cancel,
	}

	// --- 5. DHT Discovery ---
	fmt.Println("🔗 Node started:")
	for _, addr := range h.Addrs() {
		fmt.Printf("   %s/p2p/%s\n", addr, h.ID())
	}

	fmt.Println("🌐 Bootstrapping DHT...")
	disc, err := NewDiscovery(DiscoveryConfig{
		Host:       h,
		Bootstrap:  cfg.Bootstrap,
		Rendezvous: cfg.Rendezvous,
	})
	if err != nil {
		fmt.Printf("⚠️  DHT init failed: %v (direct bootstrap fallback)\n", err)
		for _, baddr := range cfg.Bootstrap {
			if ma, e := multiaddr.NewMultiaddr(baddr); e == nil {
				if info, e := peer.AddrInfoFromP2pAddr(ma); e == nil {
					h.Connect(ctx, *info)
				}
			}
		}
	} else {
		n.discovery = disc
		ns := cfg.Rendezvous
		if ns == "" {
			ns = "dmsg-v1"
		}
		disc.DiscoverPeers(ns)
		fmt.Printf("✅ DHT discovery active (namespace: %s)\n", ns)
	}

	// --- 6. Message loop ---
	n.wg.Add(1)
	go n.readLoop()

	return n, nil
}

// readLoop: full anti-abuse pipeline
// receive → dedup → rate limit → replay → PoW → signature
// → trust → abuse detect → filter → plugin → deliver
func (n *Node) readLoop() {
	defer n.wg.Done()
	for {
		raw, err := n.sub.Next(n.ctx)
		if err != nil {
			return
		}
		if raw.ReceivedFrom == n.host.ID() {
			continue
		}

		m, err := dmsgmsg.Deserialize(raw.Data)
		if err != nil {
			continue
		}

		// 1. Dedup
		if n.dedup.Seen(m.ID) {
			continue
		}

		// 2. Rate limit
		if !n.limiter.Allow(m.PubKey) {
			continue
		}

		// 3. Replay
		if err := n.replay.Check(m); err != nil {
			continue
		}

		// 4. PoW + Signature
		if err := m.Verify(n.diff); err != nil {
			continue
		}

		// 5. Trust engine
		n.trust.RecordMsg(m.PubKey)
		forwardScore := n.trust.ForwardScore(m.PubKey)
		if forwardScore < 0.01 {
			continue
		}

		// 6. Advanced abuse detection
		abuseResult := n.abuse.Check(m.ID, m.PubKey, m.Content)
		if abuseResult.IsAbuse {
			continue
		}

		// 7. Filter
		action := n.filters.Check(m)
		if action == filter.ActionBlock {
			continue
		}

		// 8. Plugin evaluation
		if n.plugins != nil {
			resp := n.plugins.Evaluate(plugin.PluginRequest{
				ID:      m.ID,
				PubKey:  m.PubKey,
				Content: m.Content,
				Timestamp: m.Timestamp,
			})
			if resp.Action == "block" {
				continue
			}
		}

		// 9. Deliver
		if n.handler != nil {
			n.handler(m)
		}
	}
}

// Publish sends a message to the network.
func (n *Node) Publish(m *dmsgmsg.Message) error {
	data, err := m.Serialize()
	if err != nil {
		return err
	}
	n.dedup.Seen(m.ID)
	return n.topic.Publish(n.ctx, data)
}

// Trust returns the trust engine.
func (n *Node) Trust() *trust.Engine { return n.trust }

// Filters returns the filter engine.
func (n *Node) Filters() *filter.Engine { return n.filters }

// Abuse returns the abuse detector.
func (n *Node) Abuse() *abuse.Detector { return n.abuse }

// Plugins returns the plugin manager (may be nil).
func (n *Node) Plugins() *plugin.Manager { return n.plugins }

// PeerCount returns connected peer count.
func (n *Node) PeerCount() int { return len(n.host.Network().Peers()) }

// Addrs returns listen addresses.
func (n *Node) Addrs() []string {
	id := n.host.ID()
	var addrs []string
	for _, a := range n.host.Addrs() {
		addrs = append(addrs, fmt.Sprintf("%s/p2p/%s", a, id))
	}
	return addrs
}

// ID returns the peer ID.
func (n *Node) ID() string { return n.host.ID().ShortString() }

// Close shuts down the node.
func (n *Node) Close() error {
	n.cancel()
	n.wg.Wait()
	n.sub.Cancel()
	n.topic.Close()
	if n.discovery != nil {
		n.discovery.Close()
	}
	return n.host.Close()
}
