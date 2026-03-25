package msg

import (
	"testing"
)

func TestPoWMineAndCheck(t *testing.T) {
	base := []byte("hello world")
	diff := 12 // 12 leading zero bits = 3 zero hex chars

	nonce, err := Mine(base, diff)
	if err != nil {
		t.Fatalf("Mine failed: %v", err)
	}

	if !CheckPoW(base, nonce, diff) {
		t.Fatal("CheckPoW failed after mining")
	}

	// Wrong nonce should fail
	if CheckPoW(base, nonce+1, diff) {
		t.Fatal("CheckPoW passed with wrong nonce")
	}
}

func TestDedupCache(t *testing.T) {
	dc := NewDedupCache(5)

	if dc.Seen("msg1") {
		t.Fatal("first see should return false")
	}
	if !dc.Seen("msg1") {
		t.Fatal("second see should return true")
	}

	// Fill cache
	for i := 0; i < 5; i++ {
		dc.Seen("msg" + string(rune('a'+i)))
	}
	if dc.Size() != 5 {
		t.Fatalf("expected size 5, got %d", dc.Size())
	}
}

func TestRateLimiter(t *testing.T) {
	rl := NewRateLimiter(1.0, 3) // 1/sec, burst 3

	// Burst of 3 should pass
	for i := 0; i < 3; i++ {
		if !rl.Allow("user1") {
			t.Fatalf("burst %d should be allowed", i)
		}
	}
	// 4th should be denied
	if rl.Allow("user1") {
		t.Fatal("4th request should be rate limited")
	}

	// Different user should pass
	if !rl.Allow("user2") {
		t.Fatal("different user should be allowed")
	}
}

func TestDynamicDifficulty(t *testing.T) {
	// Low rate = low difficulty
	d := DynamicDifficulty(10.0, 2.0)
	if d < 4 {
		t.Fatal("difficulty too low")
	}

	// High rate = high difficulty
	d2 := DynamicDifficulty(10.0, 100.0)
	if d2 <= d {
		t.Fatal("high rate should increase difficulty")
	}
	if d2 > 32 {
		t.Fatal("difficulty capped at 32")
	}
}
