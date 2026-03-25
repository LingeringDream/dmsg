package ranking

import (
	"math"
	"sort"
	"time"

	"dmsg/internal/msg"
)

// Strategy defines the ranking strategy.
type Strategy string

const (
	StrategyTime  Strategy = "time"   // newest first
	StrategyTrust Strategy = "trust"  // trust-weighted
	StrategyHot   Strategy = "hot"    // engagement (forward count)
	StrategyMixed Strategy = "mixed"  // composite score
)

// Config holds ranking parameters.
type Config struct {
	Strategy     Strategy
	TrustWeight  float64 // for mixed: weight of trust (0-1)
	TimeDecay    float64 // for mixed: half-life in hours
	TrustScores  map[string]float64 // pubkey -> trust score
	ForwardCount map[string]int     // msgID -> forward count
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		Strategy:     StrategyMixed,
		TrustWeight:  0.4,
		TimeDecay:    6.0, // 6-hour half-life
		TrustScores:  make(map[string]float64),
		ForwardCount: make(map[string]int),
	}
}

// RankedMessage wraps a message with its computed score.
type RankedMessage struct {
	Message *msg.Message
	Score   float64
}

// Rank sorts messages according to the strategy.
func Rank(messages []*msg.Message, cfg Config) []*RankedMessage {
	ranked := make([]*RankedMessage, len(messages))
	now := float64(time.Now().Unix())

	for i, m := range messages {
		score := computeScore(m, cfg, now)
		ranked[i] = &RankedMessage{Message: m, Score: score}
	}

	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].Score > ranked[j].Score
	})

	return ranked
}

func computeScore(m *msg.Message, cfg Config, now float64) float64 {
	ts := float64(m.Timestamp)
	trust := 0.5
	if t, ok := cfg.TrustScores[m.PubKey]; ok {
		trust = t
	}
	forwards := float64(cfg.ForwardCount[m.ID])

	switch cfg.Strategy {
	case StrategyTime:
		return ts

	case StrategyTrust:
		return trust*1e12 + ts // trust primary, time tiebreaker

	case StrategyHot:
		// log(forwards) to dampen viral spikes
		hot := math.Log(1 + forwards)
		return hot*1e12 + ts

	case StrategyMixed:
		// Time decay: score decays by half every TimeDecay hours
		ageHours := (now - ts) / 3600.0
		timeScore := math.Pow(0.5, ageHours/cfg.TimeDecay)

		// Trust component
		trustScore := trust

		// Hot component: log-normalized
		hotScore := math.Log(1+forwards) / 10.0
		if hotScore > 1.0 {
			hotScore = 1.0
		}

		// Composite
		w := cfg.TrustWeight
		return w*trustScore + (1-w)*0.6*timeScore + (1-w)*0.4*hotScore

	default:
		return ts
	}
}

// FilterAndRank combines filtering (trust threshold) with ranking.
func FilterAndRank(messages []*msg.Message, cfg Config, minTrust float64) []*RankedMessage {
	now := float64(time.Now().Unix())
	var ranked []*RankedMessage

	for _, m := range messages {
		trust := 0.5
		if t, ok := cfg.TrustScores[m.PubKey]; ok {
			trust = t
		}
		if trust < minTrust {
			continue
		}
		score := computeScore(m, cfg, now)
		ranked = append(ranked, &RankedMessage{Message: m, Score: score})
	}

	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].Score > ranked[j].Score
	})

	return ranked
}
