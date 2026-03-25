package filter

import (
	"testing"

	"dmsg/internal/msg"
)

func TestKeywordFilter(t *testing.T) {
	e := NewEngine()
	e.AddRule(Rule{
		Name:    "block-spam",
		Type:    "keyword",
		Match:   "viagra",
		Action:  "block",
		Enabled: true,
	})

	m := &msg.Message{Content: "buy viagra now"}
	if e.Check(m) != ActionBlock {
		t.Fatal("should block keyword match")
	}

	m2 := &msg.Message{Content: "hello world"}
	if e.Check(m2) != ActionAllow {
		t.Fatal("should allow clean message")
	}
}

func TestBlacklist(t *testing.T) {
	e := NewEngine()
	e.AddRule(Rule{
		Type:    "blacklist_pubkey",
		Match:   "abc123",
		Action:  "block",
		Enabled: true,
	})

	m := &msg.Message{PubKey: "abc123", Content: "hello"}
	if e.Check(m) != ActionBlock {
		t.Fatal("should block blacklisted pubkey")
	}

	m2 := &msg.Message{PubKey: "def456", Content: "hello"}
	if e.Check(m2) != ActionAllow {
		t.Fatal("should allow non-blacklisted pubkey")
	}
}

func TestAllowlistPriority(t *testing.T) {
	e := NewEngine()
	e.AddRule(Rule{
		Type:    "blacklist_pubkey",
		Match:   "abc123",
		Action:  "block",
		Enabled: true,
	})
	e.AddRule(Rule{
		Type:    "allowlist_pubkey",
		Match:   "abc123",
		Action:  "allow",
		Enabled: true,
	})

	m := &msg.Message{PubKey: "abc123", Content: "hello"}
	if e.Check(m) != ActionAllow {
		t.Fatal("allowlist should override blacklist")
	}
}

func TestRegexFilter(t *testing.T) {
	e := NewEngine()
	e.AddRule(Rule{
		Name:    "block-urls",
		Type:    "regex",
		Match:   `https?://[^\s]+`,
		Action:  "mute",
		Enabled: true,
	})

	m := &msg.Message{Content: "check out http://evil.com"}
	if e.Check(m) != ActionMute {
		t.Fatal("should mute URL messages")
	}

	m2 := &msg.Message{Content: "just chatting"}
	if e.Check(m2) != ActionAllow {
		t.Fatal("should allow non-URL messages")
	}
}

func TestEmptyContent(t *testing.T) {
	e := NewEngine()
	e.AddRule(Rule{
		Type:    "content_type",
		Match:   "empty",
		Action:  "block",
		Enabled: true,
	})

	m := &msg.Message{Content: "   "}
	if e.Check(m) != ActionBlock {
		t.Fatal("should block empty/whitespace content")
	}
}

func TestDefaultRules(t *testing.T) {
	e := NewEngine()
	e.SetRules(DefaultRules())

	if e.Check(&msg.Message{Content: ""}) != ActionBlock {
		t.Fatal("default rules should block empty messages")
	}
	if e.Check(&msg.Message{Content: "hi"}) != ActionAllow {
		t.Fatal("default rules should allow normal messages")
	}
}
