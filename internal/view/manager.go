package view

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"dmsg/internal/filter"
	"dmsg/internal/msg"
	"dmsg/internal/ranking"
)

// View is a named filter+rank configuration for displaying messages.
type View struct {
	Name        string              `json:"name"`
	Strategy    ranking.Strategy    `json:"strategy"`
	MinTrust    float64             `json:"min_trust"`    // 0-1, filter threshold
	TrustWeight float64             `json:"trust_weight"` // for mixed strategy
	TimeDecay   float64             `json:"time_decay"`   // half-life hours
	FollowOnly  bool                `json:"follow_only"`  // only show followed users
	FilterRules []filter.Rule       `json:"filter_rules,omitempty"`
	Limit       int                 `json:"limit"` // max messages shown
}

// DefaultViews returns a set of built-in views.
func DefaultViews() []View {
	return []View{
		{
			Name:     "All",
			Strategy: ranking.StrategyMixed,
			MinTrust: 0.0,
			Limit:    50,
		},
		{
			Name:       "Following",
			Strategy:   ranking.StrategyTime,
			MinTrust:   0.0,
			FollowOnly: true,
			Limit:      50,
		},
		{
			Name:      "High Trust",
			Strategy:  ranking.StrategyMixed,
			MinTrust:  0.7,
			Limit:     50,
		},
		{
			Name:     "Trending",
			Strategy: ranking.StrategyHot,
			MinTrust: 0.3,
			Limit:    30,
		},
		{
			Name:     "Latest",
			Strategy: ranking.StrategyTime,
			MinTrust: 0.0,
			Limit:    100,
		},
	}
}

// Manager manages multiple views and renders messages.
type Manager struct {
	mu    sync.RWMutex
	views map[string]*View
	path  string
}

// NewManager creates a view manager. Loads from file if it exists.
func NewManager(dataDir string) (*Manager, error) {
	path := filepath.Join(dataDir, "views.json")
	m := &Manager{
		views: make(map[string]*View),
		path:  path,
	}

	if _, err := os.Stat(path); err == nil {
		if err := m.Load(); err != nil {
			return nil, err
		}
	} else {
		// Set defaults
		for _, v := range DefaultViews() {
			vv := v
			m.views[v.Name] = &vv
		}
		m.Save()
	}

	return m, nil
}

// List returns all view names.
func (m *Manager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var names []string
	for n := range m.views {
		names = append(names, n)
	}
	return names
}

// Get returns a view by name.
func (m *Manager) Get(name string) *View {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.views[name]
}

// Set adds or updates a view.
func (m *Manager) Set(v View) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.views[v.Name] = &v
	m.Save()
}

// Delete removes a view.
func (m *Manager) Delete(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.views, name)
	m.Save()
}

// Render applies a view to a list of messages, returning ranked results.
// trustScores: pubkey -> trust score
// following: set of followed pubkeys
func (m *Manager) Render(viewName string, messages []*msg.Message, trustScores map[string]float64, following map[string]bool) ([]*ranking.RankedMessage, error) {
	m.mu.RLock()
	v, ok := m.views[viewName]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("view not found: %s", viewName)
	}

	// Build filter engine if rules exist
	fe := filter.NewEngine()
	if len(v.FilterRules) > 0 {
		fe.SetRules(v.FilterRules)
	}

	// Apply filters
	var filtered []*msg.Message
	for _, msg := range messages {
		// Follow-only filter
		if v.FollowOnly && !following[msg.PubKey] {
			continue
		}
		// Content filter
		if fe.Check(msg) == filter.ActionBlock {
			continue
		}
		filtered = append(filtered, msg)
	}

	// Build ranking config
	cfg := ranking.Config{
		Strategy:     v.Strategy,
		TrustWeight:  v.TrustWeight,
		TimeDecay:    v.TimeDecay,
		TrustScores:  trustScores,
		ForwardCount: make(map[string]int),
	}
	if cfg.TrustWeight == 0 {
		cfg.TrustWeight = 0.4
	}
	if cfg.TimeDecay == 0 {
		cfg.TimeDecay = 6.0
	}

	// Rank
	ranked := ranking.FilterAndRank(filtered, cfg, v.MinTrust)

	// Limit
	if v.Limit > 0 && len(ranked) > v.Limit {
		ranked = ranked[:v.Limit]
	}

	return ranked, nil
}

// Save writes views to disk.
func (m *Manager) Save() error {
	data, err := json.MarshalIndent(m.views, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.path, data, 0644)
}

// Load reads views from disk.
func (m *Manager) Load() error {
	data, err := os.ReadFile(m.path)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return json.Unmarshal(data, &m.views)
}

// ExportExports exports views to a portable JSON string.
func (m *Manager) ExportJSON() (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, err := json.MarshalIndent(m.views, "", "  ")
	return string(data), err
}

// ImportJSON imports views from a JSON string (merges, doesn't replace).
func (m *Manager) ImportJSON(jsonStr string) error {
	var imported map[string]*View
	if err := json.Unmarshal([]byte(jsonStr), &imported); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, v := range imported {
		m.views[k] = v
	}
	return m.Save()
}
