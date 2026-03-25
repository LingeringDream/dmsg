package ranking

import (
	"testing"
	"time"

	"dmsg/internal/msg"
)

func makeMsg(pubkey, content string, ts int64) *msg.Message {
	m := &msg.Message{
		ID:        pubkey + content,
		PubKey:    pubkey,
		Content:   content,
		Timestamp: ts,
	}
	return m
}

func TestTimeRanking(t *testing.T) {
	now := time.Now().Unix()
	msgs := []*msg.Message{
		makeMsg("a", "old", now-3600),
		makeMsg("b", "new", now),
		makeMsg("c", "mid", now-1800),
	}

	cfg := Config{
		Strategy:     StrategyTime,
		TrustScores:  make(map[string]float64),
		ForwardCount: make(map[string]int),
	}
	ranked := Rank(msgs, cfg)

	if ranked[0].Message.Content != "new" {
		t.Fatalf("expected 'new' first, got '%s'", ranked[0].Message.Content)
	}
	if ranked[2].Message.Content != "old" {
		t.Fatalf("expected 'old' last, got '%s'", ranked[2].Message.Content)
	}
}

func TestTrustRanking(t *testing.T) {
	now := time.Now().Unix()
	msgs := []*msg.Message{
		makeMsg("trusted", "hello", now-60),
		makeMsg("unknown", "hello", now),
	}

	cfg := Config{
		Strategy:     StrategyTrust,
		TrustScores:  map[string]float64{"trusted": 1.0, "unknown": 0.1},
		ForwardCount: make(map[string]int),
	}
	ranked := Rank(msgs, cfg)

	if ranked[0].Message.PubKey != "trusted" {
		t.Fatal("trusted user should rank higher")
	}
}

func TestHotRanking(t *testing.T) {
	now := time.Now().Unix()
	msgs := []*msg.Message{
		makeMsg("a", "viral", now-60),
		makeMsg("b", "boring", now),
	}

	cfg := Config{
		Strategy:     StrategyHot,
		TrustScores:  make(map[string]float64),
		ForwardCount: map[string]int{"aviral": 1000, "bboring": 1},
	}
	ranked := Rank(msgs, cfg)

	if ranked[0].Message.Content != "viral" {
		t.Fatal("high-forward message should rank first in hot strategy")
	}
}

func TestMixedRanking(t *testing.T) {
	now := time.Now().Unix()
	msgs := []*msg.Message{
		makeMsg("trusted", "recent", now-60),
		makeMsg("unknown", "brand new", now),
		makeMsg("muted", "spam", now),
	}

	cfg := DefaultConfig()
	cfg.TrustScores = map[string]float64{
		"trusted": 1.0,
		"unknown": 0.5,
		"muted":   0.0,
	}
	ranked := Rank(msgs, cfg)

	// Trusted recent should rank high
	if ranked[0].Message.PubKey != "trusted" {
		t.Fatalf("expected trusted+recent first, got %s", ranked[0].Message.PubKey)
	}
}

func TestFilterAndRank(t *testing.T) {
	now := time.Now().Unix()
	msgs := []*msg.Message{
		makeMsg("high", "good", now),
		makeMsg("low", "bad", now),
	}

	cfg := Config{
		Strategy:     StrategyTime,
		TrustScores:  map[string]float64{"high": 0.8, "low": 0.1},
		ForwardCount: make(map[string]int),
	}

	ranked := FilterAndRank(msgs, cfg, 0.5)
	if len(ranked) != 1 {
		t.Fatalf("expected 1 message after trust filter, got %d", len(ranked))
	}
	if ranked[0].Message.PubKey != "high" {
		t.Fatal("should keep high-trust message")
	}
}
