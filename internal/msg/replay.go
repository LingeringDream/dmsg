package msg

import (
	"sync"
	"time"
)

// ReplayGuard rejects messages that are too old or have been seen recently.
type ReplayGuard struct {
	mu          sync.Mutex
	maxAge      time.Duration // max age of a message
	recentIDs   *DedupCache   // LRU of recently seen IDs
	recentSigs  *DedupCache   // LRU of recently seen signatures
}

// NewReplayGuard creates a guard with the given max message age and cache size.
func NewReplayGuard(maxAge time.Duration, cacheSize int) *ReplayGuard {
	return &ReplayGuard{
		maxAge:     maxAge,
		recentIDs:  NewDedupCache(cacheSize),
		recentSigs: NewDedupCache(cacheSize),
	}
}

// Check returns nil if the message passes replay checks.
func (rg *ReplayGuard) Check(m *Message) error {
	// 1. Timestamp freshness
	now := time.Now().Unix()
	if now-m.Timestamp > int64(rg.maxAge.Seconds()) {
		return ErrTooOld
	}
	if m.Timestamp > now+300 { // 5 min clock skew tolerance
		return ErrFromFuture
	}

	// 2. ID dedup
	if rg.recentIDs.Seen(m.ID) {
		return ErrDuplicate
	}

	// 3. Signature dedup (catches same content re-signed, unlikely but safe)
	if rg.recentSigs.Seen(m.Signature) {
		return ErrDuplicate
	}

	return nil
}

var (
	ErrTooOld    = &ReplayError{"message too old"}
	ErrFromFuture = &ReplayError{"message from the future"}
	ErrDuplicate = &ReplayError{"duplicate message"}
)

type ReplayError struct{ msg string }

func (e *ReplayError) Error() string { return e.msg }
