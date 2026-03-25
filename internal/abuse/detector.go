package abuse

import (
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// Detector implements advanced anti-abuse checks.
type Detector struct {
	mu            sync.RWMutex
	fingerprints  *FingerprintDB
	sybil         *SybilTracker
	anomaly       *AnomalyTracker
}

// NewDetector creates a new abuse detector.
func NewDetector() *Detector {
	return &Detector{
		fingerprints: NewFingerprintDB(50_000),
		sybil:        NewSybilTracker(),
		anomaly:      NewAnomalyTracker(),
	}
}

// --- Content Fingerprinting (near-duplicate detection) ---

// FingerprintDB detects near-duplicate content.
type FingerprintDB struct {
	mu      sync.RWMutex
	shingles map[string][]string // msgID -> shingle set
	index   map[string][]string // shingle -> msgIDs
	capacity int
	order   []string
}

func NewFingerprintDB(capacity int) *FingerprintDB {
	return &FingerprintDB{
		shingles: make(map[string][]string),
		index:    make(map[string][]string),
		capacity: capacity,
	}
}

// Shingle produces k-shingles from text (k=3 word shingles).
func Shingle(text string, k int) []string {
	words := strings.Fields(strings.ToLower(text))
	if len(words) < k {
		return []string{strings.Join(words, " ")}
	}
	seen := make(map[string]bool)
	var shingles []string
	for i := 0; i <= len(words)-k; i++ {
		s := strings.Join(words[i:i+k], " ")
		if !seen[s] {
			seen[s] = true
			shingles = append(shingles, s)
		}
	}
	return shingles
}

// CheckDuplicate returns a similarity score [0,1] against recent messages.
// Score > 0.7 means likely duplicate/spam.
func (f *FingerprintDB) CheckDuplicate(id, content string) float64 {
	shingles := Shingle(content, 3)
	if len(shingles) == 0 {
		return 0
	}

	f.mu.RLock()
	defer f.mu.RUnlock()

	// Find messages sharing shingles
	candidates := make(map[string]int) // msgID -> shared count
	for _, s := range shingles {
		for _, mid := range f.index[s] {
			if mid != id {
				candidates[mid]++
			}
		}
	}

	if len(candidates) == 0 {
		return 0
	}

	// Find max Jaccard similarity
	maxSim := 0.0
	for mid, shared := range candidates {
		otherShingles := len(f.shingles[mid])
		union := len(shingles) + otherShingles - shared
		if union > 0 {
			sim := float64(shared) / float64(union)
			if sim > maxSim {
				maxSim = sim
			}
		}
	}
	return maxSim
}

// AddFingerprint stores a message's fingerprint.
func (f *FingerprintDB) AddFingerprint(id, content string) {
	shingles := Shingle(content, 3)

	f.mu.Lock()
	defer f.mu.Unlock()

	// Evict if needed
	if len(f.order) >= f.capacity {
		oldest := f.order[0]
		f.order = f.order[1:]
		for _, s := range f.shingles[oldest] {
			f.index[s] = removeString(f.index[s], oldest)
			if len(f.index[s]) == 0 {
				delete(f.index, s)
			}
		}
		delete(f.shingles, oldest)
	}

	f.shingles[id] = shingles
	f.order = append(f.order, id)
	for _, s := range shingles {
		f.index[s] = append(f.index[s], id)
	}
}

func removeString(slice []string, s string) []string {
	var result []string
	for _, v := range slice {
		if v != s {
			result = append(result, v)
		}
	}
	return result
}

// --- Sybil Detection ---

// SybilTracker detects potential Sybil clusters.
type SybilTracker struct {
	mu       sync.RWMutex
	profiles map[string]*SybilProfile
}

type SybilProfile struct {
	PubKey        string
	CreatedAt     time.Time
	MsgTimes      []time.Time
	ContentLens   []int
	UniqueTargets int // how many other pubkeys they interact with
	SybilScore    float64
}

func NewSybilTracker() *SybilTracker {
	return &SybilTracker{
		profiles: make(map[string]*SybilProfile),
	}
}

// Record records a message for Sybil analysis.
func (st *SybilTracker) Record(pubkey string, contentLen int) {
	st.mu.Lock()
	defer st.mu.Unlock()

	p, ok := st.profiles[pubkey]
	if !ok {
		p = &SybilProfile{
			PubKey:    pubkey,
			CreatedAt: time.Now(),
		}
		st.profiles[pubkey] = p
	}

	now := time.Now()
	p.MsgTimes = append(p.MsgTimes, now)
	p.ContentLens = append(p.ContentLens, contentLen)

	// Keep last 100
	if len(p.MsgTimes) > 100 {
		p.MsgTimes = p.MsgTimes[len(p.MsgTimes)-100:]
		p.ContentLens = p.ContentLens[len(p.ContentLens)-100:]
	}
}

// Score returns the Sybil score [0,1] for a pubkey.
// Higher = more likely Sybil.
func (st *SybilTracker) Score(pubkey string) float64 {
	st.mu.RLock()
	defer st.mu.RUnlock()

	p, ok := st.profiles[pubkey]
	if !ok || len(p.MsgTimes) < 3 {
		return 0
	}

	var score float64

	// Factor 1: Account age (newer = more suspicious)
	age := time.Since(p.CreatedAt).Hours()
	if age < 1 {
		score += 0.3
	} else if age < 24 {
		score += 0.15
	}

	// Factor 2: Message regularity (too regular = bot-like)
	if len(p.MsgTimes) >= 5 {
		var intervals []float64
		for i := 1; i < len(p.MsgTimes); i++ {
			intervals = append(intervals, p.MsgTimes[i].Sub(p.MsgTimes[i-1]).Seconds())
		}
		stddev := stdDev(intervals)
		mean := mean(intervals)
		if mean > 0 {
			cv := stddev / mean // coefficient of variation
			if cv < 0.1 {       // very regular timing
				score += 0.3
			} else if cv < 0.3 {
				score += 0.15
			}
		}
	}

	// Factor 3: Content length variance (too uniform = bot)
	if len(p.ContentLens) >= 5 {
		lensFloat := make([]float64, len(p.ContentLens))
		for i, l := range p.ContentLens {
			lensFloat[i] = float64(l)
		}
		stddev := stdDev(lensFloat)
		if stddev < 5 { // very uniform lengths
			score += 0.2
		}
	}

	// Factor 4: Burst creation (many msgs shortly after account creation)
	if len(p.MsgTimes) >= 10 {
		firstBatch := p.MsgTimes[9].Sub(p.CreatedAt).Minutes()
		if firstBatch < 5 { // 10 msgs in 5 minutes of creation
			score += 0.3
		}
	}

	if score > 1.0 {
		score = 1.0
	}
	p.SybilScore = score
	return score
}

// --- Anomaly Detection ---

// AnomalyTracker detects rate and pattern anomalies.
type AnomalyTracker struct {
	mu       sync.RWMutex
	counters map[string]*WindowCounter
}

type WindowCounter struct {
	Timestamps []time.Time
	WindowSize time.Duration
}

func NewAnomalyTracker() *AnomalyTracker {
	return &AnomalyTracker{
		counters: make(map[string]*WindowCounter),
	}
}

// CheckRate returns the message rate (msgs/min) for a pubkey in the recent window.
func (at *AnomalyTracker) CheckRate(pubkey string) float64 {
	at.mu.Lock()
	defer at.mu.Unlock()

	now := time.Now()
	wc, ok := at.counters[pubkey]
	if !ok {
		wc = &WindowCounter{WindowSize: 5 * time.Minute}
		at.counters[pubkey] = wc
	}

	// Add current timestamp
	wc.Timestamps = append(wc.Timestamps, now)

	// Prune old
	cutoff := now.Add(-wc.WindowSize)
	var fresh []time.Time
	for _, ts := range wc.Timestamps {
		if ts.After(cutoff) {
			fresh = append(fresh, ts)
		}
	}
	wc.Timestamps = fresh

	if len(fresh) < 2 {
		return 0
	}

	duration := fresh[len(fresh)-1].Sub(fresh[0]).Minutes()
	if duration < 0.01 {
		duration = 0.01
	}
	return float64(len(fresh)) / duration
}

// Cleanup removes stale counters.
func (at *AnomalyTracker) Cleanup() {
	at.mu.Lock()
	defer at.mu.Unlock()
	cutoff := time.Now().Add(-10 * time.Minute)
	for k, wc := range at.counters {
		allOld := true
		for _, ts := range wc.Timestamps {
			if ts.After(cutoff) {
				allOld = false
				break
			}
		}
		if allOld {
			delete(at.counters, k)
		}
	}
}

// --- Public API ---

// CheckResult is the result of an abuse check.
type CheckResult struct {
	IsAbuse       bool
	Score         float64
	DuplicateSim  float64
	SybilScore    float64
	RatePerMinute float64
	Reasons       []string
}

// Check runs all abuse detectors on a message.
func (d *Detector) Check(id, pubkey, content string) CheckResult {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var result CheckResult

	// 1. Content fingerprint
	result.DuplicateSim = d.fingerprints.CheckDuplicate(id, content)
	if result.DuplicateSim > 0.7 {
		result.Reasons = append(result.Reasons, "near-duplicate content")
	}

	// 2. Sybil detection
	d.sybil.Record(pubkey, len(content))
	result.SybilScore = d.sybil.Score(pubkey)
	if result.SybilScore > 0.6 {
		result.Reasons = append(result.Reasons, "suspicious account pattern")
	}

	// 3. Rate anomaly
	result.RatePerMinute = d.anomaly.CheckRate(pubkey)
	if result.RatePerMinute > 30 {
		result.Reasons = append(result.Reasons, "abnormal message rate")
	}

	// Composite score
	result.Score = math.Max(result.DuplicateSim, math.Max(result.SybilScore, result.RatePerMinute/100))
	if result.Score > 1.0 {
		result.Score = 1.0
	}
	result.IsAbuse = result.Score > 0.6

	// Store fingerprint for future dedup
	d.fingerprints.AddFingerprint(id, content)

	return result
}

// Cleanup runs periodic cleanup on all sub-detectores.
func (d *Detector) Cleanup() {
	d.anomaly.Cleanup()
}

// --- Helpers ---

func mean(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func stdDev(vals []float64) float64 {
	if len(vals) < 2 {
		return 0
	}
	m := mean(vals)
	variance := 0.0
	for _, v := range vals {
		d := v - m
		variance += d * d
	}
	return math.Sqrt(variance / float64(len(vals)-1))
}

// Percentile returns the p-th percentile of values.
func percentile(vals []float64, p float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)
	idx := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	return sorted[idx]
}
