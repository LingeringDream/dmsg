package trust

import (
	"math"
	"sync"
	"time"
)

// Engine manages trust scores for pubkeys.
// trust(A,B) = direct + α × indirect, with decay by depth.
type Engine struct {
	mu      sync.RWMutex
	direct  map[string]float64     // pubkey -> direct trust (follows)
	indirect map[string][]indirectEntry // pubkey -> list of indirect trust sources
	follows map[string]bool        // pubkeys we directly follow
	alpha   float64               // indirect trust weight
	decay   float64               // per-hop decay factor
	history map[string]*behaviorTracker // behavior tracking
}

type indirectEntry struct {
	source string  // who vouched
	score  float64 // their trust in target
	depth  int     // hop depth
}

type behaviorTracker struct {
	msgCount    int
	lastMsgTime time.Time
	burstCount  int
	burstWindow time.Time
}

// NewEngine creates a new trust engine.
func NewEngine() *Engine {
	return &Engine{
		direct:   make(map[string]float64),
		indirect: make(map[string][]indirectEntry),
		follows:  make(map[string]bool),
		alpha:    0.5,
		decay:    0.5,
		history:  make(map[string]*behaviorTracker),
	}
}

// Follow sets direct trust to 1.0 for a pubkey (explicit follow).
func (e *Engine) Follow(pubkey string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.direct[pubkey] = 1.0
	e.follows[pubkey] = true
}

// Unfollow removes direct trust for a pubkey.
func (e *Engine) Unfollow(pubkey string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.direct, pubkey)
	delete(e.follows, pubkey)
}

// Mute sets direct trust to 0.0 for a pubkey.
func (e *Engine) Mute(pubkey string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.direct[pubkey] = 0.0
	e.follows[pubkey] = false
}

// IsFollowing returns whether we directly follow this pubkey.
func (e *Engine) IsFollowing(pubkey string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.follows[pubkey]
}

// List returns all followed pubkeys.
func (e *Engine) List() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var result []string
	for pk := range e.follows {
		result = append(result, pk)
	}
	return result
}

// AddIndirectTrust records that `source` trusts `target` with the given score.
// Uses α^depth decay.
func (e *Engine) AddIndirectTrust(source, target string, score float64, depth int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.indirect[target] = append(e.indirect[target], indirectEntry{
		source: source,
		score:  score,
		depth:  depth,
	})
}

// RecordMsg records that a pubkey sent a message (for behavior tracking).
func (e *Engine) RecordMsg(pubkey string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()
	bt, ok := e.history[pubkey]
	if !ok {
		bt = &behaviorTracker{}
		e.history[pubkey] = bt
	}
	bt.msgCount++
	bt.lastMsgTime = now

	// Burst detection: if 10+ messages in 10 seconds
	if now.Sub(bt.burstWindow) < 10*time.Second {
		bt.burstCount++
	} else {
		bt.burstCount = 1
		bt.burstWindow = now
	}
}

// Score returns the trust score for a pubkey ∈ [0, 1].
// Formula: direct + α × Σ(indirect × decay^depth), clamped to [0,1].
// Behavioral penalties applied on top.
func (e *Engine) Score(pubkey string) float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Direct trust
	direct := 0.5 // default neutral
	if d, ok := e.direct[pubkey]; ok {
		direct = d
	}

	// Indirect trust (with decay)
	var indirectSum float64
	for _, entry := range e.indirect[pubkey] {
		factor := math.Pow(e.decay, float64(entry.depth))
		indirectSum += entry.score * factor
	}

	raw := direct + e.alpha*indirectSum

	// Behavioral penalty
	if bt, ok := e.history[pubkey]; ok {
		if bt.burstCount > 10 {
			raw *= 0.3 // heavy burst penalty
		}
	}

	// Clamp
	if raw > 1.0 {
		raw = 1.0
	}
	if raw < 0.0 {
		raw = 0.0
	}
	return raw
}

// ForwardScore computes the forwarding score for a message:
// forward_score = trust × (1 / frequency)
// Higher = more likely to forward.
func (e *Engine) ForwardScore(pubkey string) float64 {
	trust := e.Score(pubkey)
	e.mu.RLock()
	bt, ok := e.history[pubkey]
	e.mu.RUnlock()
	if !ok {
		return trust
	}

	// Frequency: messages per minute (approximate)
	age := time.Since(bt.burstWindow).Minutes()
	if age < 0.01 {
		age = 0.01
	}
	freq := float64(bt.msgCount) / age
	if freq < 0.1 {
		freq = 0.1
	}

	return trust / freq
}

// Cleanup removes stale indirect trust entries and behavior history.
func (e *Engine) Cleanup(maxAge time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	cutoff := time.Now().Add(-maxAge)
	for pubkey, bt := range e.history {
		if bt.lastMsgTime.Before(cutoff) {
			delete(e.history, pubkey)
		}
	}
}
