package trust

import (
	"testing"
)

func TestFollowUnfollow(t *testing.T) {
	e := NewEngine()

	// Default neutral score
	if s := e.Score("alice"); s != 0.5 {
		t.Fatalf("expected 0.5, got %f", s)
	}

	// Follow → trust = 1.0
	e.Follow("alice")
	if s := e.Score("alice"); s != 1.0 {
		t.Fatalf("expected 1.0 after follow, got %f", s)
	}
	if !e.IsFollowing("alice") {
		t.Fatal("should be following")
	}

	// Unfollow → back to neutral
	e.Unfollow("alice")
	if e.IsFollowing("alice") {
		t.Fatal("should not be following after unfollow")
	}
}

func TestMute(t *testing.T) {
	e := NewEngine()
	e.Mute("spammer")
	if s := e.Score("spammer"); s != 0.0 {
		t.Fatalf("expected 0.0 after mute, got %f", s)
	}
}

func TestIndirectTrust(t *testing.T) {
	e := NewEngine()
	e.Follow("alice") // we trust alice (direct = 1.0)

	// Alice vouches for bob (depth 1)
	e.AddIndirectTrust("alice", "bob", 0.8, 1)
	// score = direct(0.5) + α(0.5) × indirect(0.8 × 0.5^1) = 0.5 + 0.2 = 0.7
	s := e.Score("bob")
	if s < 0.6 || s > 0.8 {
		t.Fatalf("expected ~0.7, got %f", s)
	}
}

func TestIndirectTrustDecay(t *testing.T) {
	e := NewEngine()

	// Direct trust from trusted peer
	e.AddIndirectTrust("trusted", "target", 0.8, 1)
	s1 := e.Score("target")

	// Same trust but deeper (should decay)
	e2 := NewEngine()
	e2.AddIndirectTrust("trusted", "target", 0.8, 3)
	s2 := e2.Score("target")

	if s1 <= s2 {
		t.Fatalf("deeper depth should decay more: s1=%f s2=%f", s1, s2)
	}
}

func TestForwardScore(t *testing.T) {
	e := NewEngine()
	e.Follow("alice")

	// Record some messages
	for i := 0; i < 5; i++ {
		e.RecordMsg("alice")
	}

	fs := e.ForwardScore("alice")
	if fs <= 0 {
		t.Fatal("forward score should be positive")
	}

	// Muted user should have 0 forward score
	e.Mute("spammer")
	for i := 0; i < 100; i++ {
		e.RecordMsg("spammer")
	}
	fs2 := e.ForwardScore("spammer")
	if fs2 != 0 {
		t.Fatalf("muted user should have 0 forward score, got %f", fs2)
	}
}

func TestBurstPenalty(t *testing.T) {
	e := NewEngine()
	e.Follow("alice")

	// Normal usage
	e.RecordMsg("alice")
	s1 := e.Score("alice")

	// Burst: 20 messages in quick succession
	for i := 0; i < 20; i++ {
		e.RecordMsg("alice")
	}
	s2 := e.Score("alice")

	if s2 >= s1 {
		t.Fatalf("burst should reduce score: before=%f after=%f", s1, s2)
	}
}

func TestScoreBounds(t *testing.T) {
	e := NewEngine()

	// Extreme indirect trust shouldn't exceed 1.0
	for i := 0; i < 100; i++ {
		e.AddIndirectTrust("voter"+string(rune(i)), "target", 1.0, 1)
	}
	if s := e.Score("target"); s > 1.0 {
		t.Fatalf("score should be clamped to 1.0, got %f", s)
	}
}
