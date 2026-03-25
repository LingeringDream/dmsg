package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// Identity holds an Ed25519 key pair and derived user ID.
type Identity struct {
	PrivKey  ed25519.PrivateKey
	PubKey   ed25519.PublicKey
	UserID   string // hex(sha256(pubkey))
	keyPath  string
}

// Generate creates a new random identity.
func Generate() (*Identity, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("keygen: %w", err)
	}
	return &Identity{
		PrivKey: priv,
		PubKey:  pub,
		UserID:  hashPubKey(pub),
	}, nil
}

// LoadOrGenerate loads an existing key from dir or creates a new one.
// Saves the private key as dir/key; derives pubkey and userID.
func LoadOrGenerate(dir string) (*Identity, error) {
	keyPath := filepath.Join(dir, "key")
	data, err := os.ReadFile(keyPath)
	if err == nil && len(data) == ed25519.PrivateKeySize {
		priv := ed25519.PrivateKey(data)
		pub := priv.Public().(ed25519.PublicKey)
		return &Identity{
			PrivKey: priv,
			PubKey:  pub,
			UserID:  hashPubKey(pub),
			keyPath: keyPath,
		}, nil
	}

	// Generate new
	id, err := Generate()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("mkdir: %w", err)
	}
	if err := os.WriteFile(keyPath, id.PrivKey, 0600); err != nil {
		return nil, fmt.Errorf("save key: %w", err)
	}
	id.keyPath = keyPath
	return id, nil
}

// Sign signs the given message bytes.
func (id *Identity) Sign(msg []byte) []byte {
	return ed25519.Sign(id.PrivKey, msg)
}

// Verify checks a signature against a pubkey.
func Verify(pub ed25519.PublicKey, msg, sig []byte) bool {
	return ed25519.Verify(pub, msg, sig)
}

// VerifyFromHex verifies using hex-encoded pubkey, msg, and signature.
func VerifyFromHex(pubHex string, msg []byte, sigHex string) (bool, error) {
	pubBytes, err := hex.DecodeString(pubHex)
	if err != nil {
		return false, fmt.Errorf("decode pubkey: %w", err)
	}
	sigBytes, err := hex.DecodeString(sigHex)
	if err != nil {
		return false, fmt.Errorf("decode sig: %w", err)
	}
	return ed25519.Verify(ed25519.PublicKey(pubBytes), msg, sigBytes), nil
}

func hashPubKey(pub ed25519.PublicKey) string {
	h := sha256.Sum256(pub)
	return hex.EncodeToString(h[:])
}

// PubKeyHex returns the public key as hex string.
func (id *Identity) PubKeyHex() string {
	return hex.EncodeToString(id.PubKey)
}
