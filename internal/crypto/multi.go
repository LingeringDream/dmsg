package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const defaultKeyDir = "keys"

// MultiIdentity manages multiple Ed25519 identities.
type MultiIdentity struct {
	Identities map[string]*Identity // userID -> Identity
	Active     *Identity
	keyDir     string
}

// NewMultiIdentity loads all identities from dir, or creates a default one.
func NewMultiIdentity(baseDir string) (*MultiIdentity, error) {
	keyDir := filepath.Join(baseDir, defaultKeyDir)
	if err := os.MkdirAll(keyDir, 0700); err != nil {
		return nil, fmt.Errorf("mkdir keys: %w", err)
	}

	mi := &MultiIdentity{
		Identities: make(map[string]*Identity),
		keyDir:     keyDir,
	}

	// Scan existing key files
	entries, err := os.ReadDir(keyDir)
	if err != nil {
		return nil, err
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".key") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(keyDir, e.Name()))
		if err != nil || len(data) != ed25519.PrivateKeySize {
			continue
		}
		priv := ed25519.PrivateKey(data)
		pub := priv.Public().(ed25519.PublicKey)
		id := &Identity{
			PrivKey: priv,
			PubKey:  pub,
			UserID:  hashPubKey(pub),
			keyPath: filepath.Join(keyDir, e.Name()),
		}
		mi.Identities[id.UserID] = id
	}

	// Create default if none exist
	if len(mi.Identities) == 0 {
		id, err := mi.Create()
		if err != nil {
			return nil, err
		}
		mi.Active = id
	} else {
		// Use first as active
		for _, id := range mi.Identities {
			mi.Active = id
			break
		}
	}

	return mi, nil
}

// Create generates a new identity and saves it.
func (mi *MultiIdentity) Create() (*Identity, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("keygen: %w", err)
	}

	name := hashPubKey(pub)[:8]
	keyPath := filepath.Join(mi.keyDir, name+".key")
	if err := os.WriteFile(keyPath, priv, 0600); err != nil {
		return nil, fmt.Errorf("save key: %w", err)
	}

	id := &Identity{
		PrivKey: priv,
		PubKey:  pub,
		UserID:  hashPubKey(pub),
		keyPath: keyPath,
	}
	mi.Identities[id.UserID] = id
	return id, nil
}

// Switch sets the active identity by userID prefix.
func (mi *MultiIdentity) Switch(prefix string) (*Identity, error) {
	for uid, id := range mi.Identities {
		if strings.HasPrefix(uid, prefix) {
			mi.Active = id
			return id, nil
		}
	}
	return nil, fmt.Errorf("no identity matching prefix: %s", prefix)
}

// List returns all identity user IDs.
func (mi *MultiIdentity) List() []string {
	var ids []string
	for uid := range mi.Identities {
		mi.Identities[uid]
		ids = append(ids, uid)
	}
	return ids
}
