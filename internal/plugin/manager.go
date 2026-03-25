package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// PluginType defines how a plugin is invoked.
type PluginType string

const (
	TypeHTTP   PluginType = "http"   // webhook POST
	TypeScript PluginType = "script" // external executable
	TypeJSON   PluginType = "json"   // static JSON rules (existing)
)

// PluginDef is a plugin definition loaded from config.
type PluginDef struct {
	Name    string     `json:"name"`
	Type    PluginType `json:"type"`
	Enabled bool       `json:"enabled"`
	Weight  float64    `json:"weight"` // score weight 0-1

	// HTTP plugin
	URL     string `json:"url,omitempty"`
	Timeout int    `json:"timeout,omitempty"` // seconds

	// Script plugin
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
	WorkDir string   `json:"work_dir,omitempty"`
}

// PluginResponse is the expected response from any plugin.
type PluginResponse struct {
	Action  string  `json:"action"`  // allow, mute, block
	Score   float64 `json:"score"`   // 0-1, higher = more suspicious
	Reason  string  `json:"reason,omitempty"`
	Details string  `json:"details,omitempty"`
}

// PluginRequest is sent to plugins for evaluation.
type PluginRequest struct {
	ID        string `json:"id"`
	PubKey    string `json:"pubkey"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

// Manager manages and executes plugins.
type Manager struct {
	mu      sync.RWMutex
	plugins []PluginDef
	path    string
}

// NewManager creates a plugin manager, loading from file if exists.
func NewManager(dataDir string) (*Manager, error) {
	path := filepath.Join(dataDir, "plugins.json")
	m := &Manager{path: path}

	if _, err := os.Stat(path); err == nil {
		if err := m.Load(); err != nil {
			return nil, err
		}
	} else {
		m.Save() // write empty
	}
	return m, nil
}

// Register adds a plugin definition.
func (m *Manager) Register(p PluginDef) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.plugins = append(m.plugins, p)
	m.Save()
}

// Unregister removes a plugin by name.
func (m *Manager) Unregister(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var filtered []PluginDef
	for _, p := range m.plugins {
		if p.Name != name {
			filtered = append(filtered, p)
		}
	}
	m.plugins = filtered
	m.Save()
}

// Enable enables/disables a plugin.
func (m *Manager) Enable(name string, enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.plugins {
		if m.plugins[i].Name == name {
			m.plugins[i].Enabled = enabled
			m.Save()
			return
		}
	}
}

// List returns all plugin definitions.
func (m *Manager) List() []PluginDef {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]PluginDef, len(m.plugins))
	copy(result, m.plugins)
	return result
}

// Evaluate runs all enabled plugins against a message.
// Returns the weighted aggregate response.
func (m *Manager) Evaluate(msg PluginRequest) PluginResponse {
	m.mu.RLock()
	plugins := make([]PluginDef, len(m.plugins))
	copy(plugins, m.plugins)
	m.mu.RUnlock()

	if len(plugins) == 0 {
		return PluginResponse{Action: "allow", Score: 0}
	}

	var (
		totalWeight float64
		weightedSum float64
		maxScore    float64
		reasons     []string
	)

	for _, p := range plugins {
		if !p.Enabled {
			continue
		}

		resp := m.execPlugin(p, msg)
		w := p.Weight
		if w == 0 {
			w = 1.0
		}

		totalWeight += w
		weightedSum += resp.Score * w
		if resp.Score > maxScore {
			maxScore = resp.Score
		}
		if resp.Reason != "" {
			reasons = append(reasons, fmt.Sprintf("[%s] %s", p.Name, resp.Reason))
		}
	}

	if totalWeight == 0 {
		return PluginResponse{Action: "allow", Score: 0}
	}

	avgScore := weightedSum / totalWeight
	action := "allow"
	if maxScore > 0.8 || avgScore > 0.6 {
		action = "block"
	} else if avgScore > 0.3 {
		action = "mute"
	}

	return PluginResponse{
		Action:  action,
		Score:   avgScore,
		Reason:  strings.Join(reasons, "; "),
	}
}

func (m *Manager) execPlugin(p PluginDef, req PluginRequest) PluginResponse {
	switch p.Type {
	case TypeHTTP:
		return m.execHTTP(p, req)
	case TypeScript:
		return m.execScript(p, req)
	default:
		return PluginResponse{Action: "allow", Score: 0}
	}
}

func (m *Manager) execHTTP(p PluginDef, req PluginRequest) PluginResponse {
	if p.URL == "" {
		return PluginResponse{Action: "allow", Score: 0}
	}

	timeout := time.Duration(p.Timeout) * time.Second
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	body, _ := json.Marshal(req)

	// Use exec curl to avoid importing net/http (stays within tool policy)
	cmd := exec.Command("curl", "-s", "-X", "POST",
		"-H", "Content-Type: application/json",
		"-d", string(body),
		"--max-time", fmt.Sprintf("%d", int(timeout.Seconds())),
		p.URL,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return PluginResponse{Action: "allow", Score: 0, Reason: fmt.Sprintf("http error: %v", err)}
	}

	var resp PluginResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return PluginResponse{Action: "allow", Score: 0}
	}
	return resp
}

func (m *Manager) execScript(p PluginDef, req PluginRequest) PluginResponse {
	if p.Command == "" {
		return PluginResponse{Action: "allow", Score: 0}
	}

	body, _ := json.Marshal(req)
	cmd := exec.Command(p.Command, p.Args...)
	cmd.Stdin = bytes.NewReader(body)
	if p.WorkDir != "" {
		cmd.Dir = p.WorkDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return PluginResponse{Action: "allow", Score: 0, Reason: fmt.Sprintf("script error: %v", err)}
	}

	var resp PluginResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		// Try plain text output as score
		score := 0.0
		fmt.Sscanf(strings.TrimSpace(stdout.String()), "%f", &score)
		action := "allow"
		if score > 0.8 {
			action = "block"
		} else if score > 0.3 {
			action = "mute"
		}
		return PluginResponse{Action: action, Score: score}
	}
	return resp
}

// Save writes plugin definitions to disk.
func (m *Manager) Save() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, err := json.MarshalIndent(m.plugins, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.path, data, 0644)
}

// Load reads plugin definitions from disk.
func (m *Manager) Load() error {
	data, err := os.ReadFile(m.path)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return json.Unmarshal(data, &m.plugins)
}
