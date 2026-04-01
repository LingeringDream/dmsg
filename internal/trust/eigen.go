package trust

import (
"sync"
)

type PeerTrust struct {
mu      sync.Mutex
trust   map[string]float64
dk      map[string]float64 // Direct trust vector
}

func NewPeerTrust() *PeerTrust {
return &PeerTrust{
trust:  make(map[string]float64),
dk:     make(map[string]float64),
}
}

func (t *PeerTrust) ReportInteraction(peer string, score float64) {
t.mu.Lock()
defer t.mu.Unlock()

// Normalize input to [-1, 1]
if score > 1.0 { score = 1.0 }
if score < -1.0 { score = -1.0 }

current, ok := t.dk[peer]
if !ok { current = 0.5 }

// Simple Exponential Moving Average
newTrust := (current * 0.9) + (score * 0.1)
t.dk[peer] = newTrust
}

func (t *PeerTrust) GetTrust(peer string) float64 {
t.mu.Lock()
defer t.mu.Unlock()
if v, ok := t.dk[peer]; ok { return v }
return 0.5
}
