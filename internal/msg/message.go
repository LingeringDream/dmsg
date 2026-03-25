package msg

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"dmsg/internal/crypto"
)

// Message is the core data model.
type Message struct {
	ID        string `json:"id"`        // hex(sha256(content_hash_payload))
	PubKey    string `json:"pubkey"`    // hex(ed25519 pubkey)
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
	Nonce     uint64 `json:"nonce"`
	Signature string `json:"signature"` // hex(ed25519 sig)
}

// SignableBytes returns the bytes that must be signed.
// Excludes ID and Signature (those are derived).
func (m *Message) SignableBytes() []byte {
	// Deterministic serialization: pubkey|content|timestamp|nonce
	var buf []byte
	buf = append(buf, []byte(m.PubKey)...)
	buf = append(buf, []byte(m.Content)...)
	buf = binary.BigEndian.AppendUint64(buf, uint64(m.Timestamp))
	buf = binary.BigEndian.AppendUint64(buf, m.Nonce)
	return buf
}

// PowPayload returns the bytes hashed for PoW verification.
func (m *Message) PowPayload() []byte {
	// hash(content + nonce) — simple version
	var buf []byte
	buf = append(buf, []byte(m.Content)...)
	buf = binary.BigEndian.AppendUint64(buf, m.Nonce)
	return buf
}

// ComputeID computes the message ID from its content.
func ComputeID(content string, nonce uint64) string {
	var buf []byte
	buf = append(buf, []byte(content)...)
	buf = binary.BigEndian.AppendUint64(buf, nonce)
	h := sha256.Sum256(buf)
	return hex.EncodeToString(h[:])
}

// Create builds a new message, signs it, and does PoW.
func Create(id *crypto.Identity, content string, difficulty int) (*Message, error) {
	now := time.Now().Unix()
	m := &Message{
		PubKey:    id.PubKeyHex(),
		Content:   content,
		Timestamp: now,
	}

	// Mine PoW
	nonce, err := Mine(m.PowPayload(), difficulty)
	if err != nil {
		return nil, fmt.Errorf("pow: %w", err)
	}
	m.Nonce = nonce
	m.ID = ComputeID(content, nonce)

	// Sign
	sig := id.Sign(m.SignableBytes())
	m.Signature = hex.EncodeToString(sig)

	return m, nil
}

// Verify checks signature and PoW validity.
func (m *Message) Verify(difficulty int) error {
	// 1. Check ID
	expectedID := ComputeID(m.Content, m.Nonce)
	if m.ID != expectedID {
		return fmt.Errorf("invalid id: got %s, want %s", m.ID, expectedID)
	}

	// 2. Check PoW
	if !CheckPoW(m.PowPayload(), m.Nonce, difficulty) {
		return fmt.Errorf("invalid PoW (difficulty=%d)", difficulty)
	}

	// 3. Check signature
	ok, err := crypto.VerifyFromHex(m.PubKey, m.SignableBytes(), m.Signature)
	if err != nil {
		return fmt.Errorf("sig verify error: %w", err)
	}
	if !ok {
		return fmt.Errorf("invalid signature")
	}

	return nil
}

// Serialize encodes the message to JSON bytes.
func (m *Message) Serialize() ([]byte, error) {
	return json.Marshal(m)
}

// Deserialize decodes a message from JSON bytes.
func Deserialize(data []byte) (*Message, error) {
	var m Message
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}
