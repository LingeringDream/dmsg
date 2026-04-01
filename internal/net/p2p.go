package net

import (
"context"
"time"

"github.com/libp2p/go-libp2p"
"github.com/libp2p/go-libp2p/core/host"
"github.com/libp2p/go-libp2p/core/peer"
"github.com/libp2p/go-libp2p-pubsub"
)

// PeerScoringParams defines parameters for GossipSub peer scoring
var PeerScoringParams = pubsub.PeerScoreParams{
	TopicScoreCap:    100.0,
	AppSpecificScore: func(p peer.ID) float64 { return 0 }, // Hook for external trust engine
	AppSpecificWeight: 1.0,
	IPColocationFactorWeight: -10.0,
	IPColocationFactorThreshold: 3,
	BehaviourPenaltyWeight: -5.0,
	BehaviourPenaltyThreshold: 5,
	BehaviourPenaltyDecay:  0.986,
	DecayInterval:          12 * time.Second,
	DecayToZero:            0.01,
	RetainScore:            30 * 24 * time.Hour,
}

// PeerThresholds defines gossip threshold levels
var PeerThresholds = pubsub.PeerScoreThresholds{
	GossipThreshold:             -100,
	PublishThreshold:            -500,
	GraylistThreshold:           -1000,
	AcceptPXThreshold:           100,
	OpportunisticGraftThreshold: 5,
}

type Node struct {
	Host   host.Host
	PubSub *pubsub.PubSub
}

func NewNode(ctx context.Context, appScoreFunc func(peer.ID) float64) (*Node, error) {
	// Update app-specific score function
	if appScoreFunc != nil {
		PeerScoringParams.AppSpecificScore = appScoreFunc
	}

	opts := []libp2p.Option{
		libp2p.ListenAddrStrings(
			"/ip4/0.0.0.0/tcp/0",
			"/ip4/0.0.0.0/udp/0/quic-v1",
		),
		libp2p.EnableNATService(),
		libp2p.EnableRelayService(),
	}

	h, err := libp2p.New(opts...)
	if err != nil {
		return nil, err
	}

		ps, err := pubsub.NewGossipSub(ctx, h,
			pubsub.WithFloodPublish(true),
			pubsub.WithPeerExchange(true),
			pubsub.WithPeerScore(&PeerScoringParams, &PeerThresholds),
			pubsub.WithPeerOutboundQueueSize(500),
		)
	if err != nil {
		return nil, err
	}

	return &Node{Host: h, PubSub: ps}, nil
}

// GetPeerScore retrieves current score for a peer
// Note: PubSub does not expose direct score lookup in recent API versions.
func (n *Node) GetPeerScore(p peer.ID) (float64, error) {
	return 0.0, nil
}
