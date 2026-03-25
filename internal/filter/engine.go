package filter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"dmsg/internal/msg"
)

// Action is the result of a filter check.
type Action int

const (
	ActionAllow Action = iota
	ActionMute  // don't display, but still store
	ActionBlock // reject completely
)

func (a Action) String() string {
	switch a {
	case ActionAllow:
		return "allow"
	case ActionMute:
		return "mute"
	case ActionBlock:
		return "block"
	default:
		return "unknown"
	}
}

// Rule is a single filter rule.
type Rule struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`    // keyword, blacklist, content_type, regex
	Match   string   `json:"match"`   // pattern/value to match
	Action  string   `json:"action"`  // allow, mute, block
	Enabled bool     `json:"enabled"`
	Tags    []string `json:"tags,omitempty"`
}

// RuleSet is a collection of rules loaded from a JSON file.
type RuleSet struct {
	Rules []Rule `json:"rules"`
}

// Engine evaluates messages against a set of filter rules.
type Engine struct {
	mu          sync.RWMutex
	rules       []compiledRule
	blacklist   map[string]bool // pubkey -> blocked
	allowlist   map[string]bool // pubkey -> always allow
}

type compiledRule struct {
	Rule
	compiled *regexp.Regexp
}

// NewEngine creates an empty filter engine.
func NewEngine() *Engine {
	return &Engine{
		blacklist: make(map[string]bool),
		allowlist: make(map[string]bool),
	}
}

// LoadRules loads rules from a JSON file.
func (e *Engine) LoadRules(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return e.LoadRulesJSON(data)
}

// LoadRulesJSON loads rules from JSON bytes.
func (e *Engine) LoadRulesJSON(data []byte) error {
	var rs RuleSet
	if err := json.Unmarshal(data, &rs); err != nil {
		return fmt.Errorf("parse rules: %w", err)
	}
	return e.SetRules(rs.Rules)
}

// SetRules replaces all rules.
func (e *Engine) SetRules(rules []Rule) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.rules = nil
	e.blacklist = make(map[string]bool)
	e.allowlist = make(map[string]bool)

	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		cr := compiledRule{Rule: r}

		switch r.Type {
		case "blacklist_pubkey":
			e.blacklist[r.Match] = true
			continue
		case "allowlist_pubkey":
			e.allowlist[r.Match] = true
			continue
		case "regex":
			re, err := regexp.Compile(r.Match)
			if err != nil {
				return fmt.Errorf("invalid regex %q: %w", r.Name, err)
			}
			cr.compiled = re
		}

		e.rules = append(e.rules, cr)
	}
	return nil
}

// AddRule adds a single rule programmatically.
func (e *Engine) AddRule(r Rule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if r.Type == "blacklist_pubkey" {
		e.blacklist[r.Match] = true
		return
	}
	if r.Type == "allowlist_pubkey" {
		e.allowlist[r.Match] = true
		return
	}
	if r.Type == "regex" {
		if re, err := regexp.Compile(r.Match); err == nil {
			e.rules = append(e.rules, compiledRule{Rule: r, compiled: re})
		}
		return
	}
	e.rules = append(e.rules, compiledRule{Rule: r})
}

// Check evaluates a message and returns the action to take.
// First matching rule wins. Default is Allow.
func (e *Engine) Check(m *msg.Message) Action {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Allowlist takes priority
	if e.allowlist[m.PubKey] {
		return ActionAllow
	}

	// Blacklist
	if e.blacklist[m.PubKey] {
		return ActionBlock
	}

	// Rules (first match wins)
	for _, r := range e.rules {
		if !r.Enabled {
			continue
		}
		if e.matchRule(r, m) {
			return parseAction(r.Action)
		}
	}

	return ActionAllow
}

func (e *Engine) matchRule(r compiledRule, m *msg.Message) bool {
	switch r.Type {
	case "keyword":
		return strings.Contains(
			strings.ToLower(m.Content),
			strings.ToLower(r.Match),
		)
	case "content_type":
		// Match by message length or structure
		switch r.Match {
		case "empty":
			return len(strings.TrimSpace(m.Content)) == 0
		case "short":
			return len(m.Content) < 10
		case "long":
			return len(m.Content) > 1000
		}
		return false
	case "regex":
		if r.compiled == nil {
			return false
		}
		return r.compiled.MatchString(m.Content)
	}
	return false
}

func parseAction(s string) Action {
	switch strings.ToLower(s) {
	case "block":
		return ActionBlock
	case "mute":
		return ActionMute
	default:
		return ActionAllow
	}
}

// SaveRules writes the current rules to a JSON file.
func (e *Engine) SaveRules(path string) error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var rules []Rule
	for _, cr := range e.rules {
		rules = append(rules, cr.Rule)
	}
	for pk := range e.blacklist {
		rules = append(rules, Rule{
			Name:    "blacklist:" + pk[:8],
			Type:    "blacklist_pubkey",
			Match:   pk,
			Action:  "block",
			Enabled: true,
		})
	}
	for pk := range e.allowlist {
		rules = append(rules, Rule{
			Name:    "allowlist:" + pk[:8],
			Type:    "allowlist_pubkey",
			Match:   pk,
			Action:  "allow",
			Enabled: true,
		})
	}

	rs := RuleSet{Rules: rules}
	data, err := json.MarshalIndent(rs, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// DefaultRules returns a sensible default ruleset.
func DefaultRules() []Rule {
	return []Rule{
		{
			Name:    "block-empty",
			Type:    "content_type",
			Match:   "empty",
			Action:  "block",
			Enabled: true,
		},
	}
}

// SaveDefaultRules writes default rules to the given path if it doesn't exist.
func SaveDefaultRules(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil // already exists
	}
	rs := RuleSet{Rules: DefaultRules()}
	data, err := json.MarshalIndent(rs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
