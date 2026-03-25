package net

import (
	"context"
	"sync"
	"time"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	dutil "github.com/libp2p/go-libp2p/p2p/discovery/util"
)

// Discovery wraps Kademlia DHT for peer discovery.
type Discovery struct {
	dht      *dht.IpfsDHT
	routing  *drouting.RoutingDiscovery
	host     host.Host
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// DiscoveryConfig holds discovery configuration.
type DiscoveryConfig struct {
	Host       host.Host
	Bootstrap  []string // bootstrap multiaddrs
	Rendezvous string   // discovery namespace
}

// NewDiscovery creates and bootstraps a DHT-based discovery service.
func NewDiscovery(cfg DiscoveryConfig) (*Discovery, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Create Kademlia DHT in client mode
	kdht, err := dht.New(ctx, cfg.Host, dht.Mode(dht.ModeAutoServer))
	if err != nil {
		cancel()
		return nil, err
	}

	// Bootstrap the DHT
	if err := kdht.Bootstrap(ctx); err != nil {
		cancel()
		return nil, err
	}

	// Connect to bootstrap peers for DHT seeding
	var wg sync.WaitGroup
	for _, baddr := range cfg.Bootstrap {
		wg.Add(1)
		go func(addrStr string) {
			defer wg.Done()
			ma, err := parseMultiaddr(addrStr)
			if err != nil {
				return
			}
			info, err := peer.AddrInfoFromP2pAddr(ma)
			if err != nil {
				return
			}
			cfg.Host.Connect(ctx, *info)
		}(baddr)
	}
	wg.Wait()

	// Wait for DHT routing table to populate
	time.Sleep(2 * time.Second)

	// Create routing discovery
	routing := drouting.NewRoutingDiscovery(kdht)

	d := &Discovery{
		dht:     kdht,
		routing: routing,
		host:    cfg.Host,
		ctx:     ctx,
		cancel:  cancel,
	}

	// Start advertising
	rendezvous := cfg.Rendezvous
	if rendezvous == "" {
		rendezvous = "dmsg-v1"
	}
	d.wg.Add(1)
	go d.advertiseLoop(rendezvous)

	return d, nil
}

// advertiseLoop periodically advertises our presence on the DHT.
func (d *Discovery) advertiseLoop(ns string) {
	defer d.wg.Done()
	dutil.Advertise(d.ctx, d.routing, ns)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			dutil.Advertise(d.ctx, d.routing, ns)
		}
	}
}

// FindPeers discovers peers on the DHT for a given namespace.
// Returns a channel of peer.AddrInfo.
func (d *Discovery) FindPeers(ns string) (<-chan peer.AddrInfo, error) {
	return d.routing.FindPeers(d.ctx, ns)
}

// DiscoverPeers runs a continuous discovery loop, connecting to found peers.
func (d *Discovery) DiscoverPeers(ns string) {
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		peerChan, err := d.FindPeers(ns)
		if err != nil {
			return
		}
		for p := range peerChan {
			if p.ID == d.host.ID() {
				continue
			}
			if d.host.Network().Connectedness(p.ID) == 1 {
				continue // already connected
			}
			ctx, cancel := context.WithTimeout(d.ctx, 10*time.Second)
			d.host.Connect(ctx, p)
			cancel()
		}
	}()
}

// Peers returns all connected peer IDs.
func (d *Discovery) Peers() []peer.ID {
	return d.host.Network().Peers()
}

// Close shuts down the discovery service.
func (d *Discovery) Close() error {
	d.cancel()
	d.wg.Wait()
	return d.dht.Close()
}
