package abuse

import (
	"testing"
)

func TestShingle(t *testing.T) {
	shingles := Shingle("the quick brown fox jumps over the lazy dog", 3)
	if len(shingles) == 0 {
		t.Fatal("expected shingles")
	}
	// "the quick brown" should be one
	found := false
	for _, s := range shingles {
		if s == "the quick brown" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'the quick brown' shingle")
	}
}

func TestNearDuplicateDetection(t *testing.T) {
	db := NewFingerprintDB(100)

	// Add a message
	db.AddFingerprint("msg1", "the quick brown fox jumps over the lazy dog")

	// Exact duplicate
	sim := db.CheckDuplicate("msg2", "the quick brown fox jumps over the lazy dog")
	if sim < 0.9 {
		t.Fatalf("exact duplicate should have high similarity, got %f", sim)
	}

	// Near duplicate (small edit)
	sim2 := db.CheckDuplicate("msg3", "the quick brown fox jumps over a lazy dog")
	if sim2 < 0.5 {
		t.Fatalf("near duplicate should have moderate similarity, got %f", sim2)
	}

	// Different content
	sim3 := db.CheckDuplicate("msg4", "hello world how are you today")
	if sim3 > 0.3 {
		t.Fatalf("different content should have low similarity, got %f", sim3)
	}
}

func TestSybilDetection(t *testing.T) {
	st := NewSybilTracker()

	// Normal user: sparse messages over time
	for i := 0; i < 5; i++ {
		st.Record("normal", 50+i*10)
	}
	normalScore := st.Score("normal")
	if normalScore > 0.5 {
		t.Fatalf("normal user should have low sybil score, got %f", normalScore)
	}
}

func TestAnomalyRateCheck(t *testing.T) {
	at := NewAnomalyTracker()

	// Normal rate
	for i := 0; i < 5; i++ {
		at.CheckRate("user1")
	}
	rate := at.CheckRate("user1")
	if rate > 100 {
		t.Fatalf("expected reasonable rate, got %f", rate)
	}
}

func TestDetectorIntegration(t *testing.T) {
	d := NewDetector()

	// First message
	r1 := d.Check("msg1", "user1", "hello world this is a test")
	if r1.IsAbuse {
		t.Fatal("first message should not be abuse")
	}

	// Duplicate
	r2 := d.Check("msg2", "user1", "hello world this is a test")
	if r2.DuplicateSim < 0.9 {
		t.Fatalf("duplicate should have high sim, got %f", r2.DuplicateSim)
	}

	// Different user, different content
	r3 := d.Check("msg3", "user2", "completely different message here")
	if r3.IsAbuse {
		t.Fatal("different content should not be abuse")
	}
}
