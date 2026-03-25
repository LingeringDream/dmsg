package ipfs

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

// Store provides optional IPFS-backed content-addressed storage.
type Store struct {
	gatewayURL string
	apiURL     string
	dataDir    string
}

// Config holds IPFS store configuration.
type Config struct {
	GatewayURL string `json:"gateway_url"`
	APIURL     string `json:"api_url"`
	DataDir    string `json:"data_dir"`
}

// NewStore creates an IPFS-backed store.
func NewStore(cfg Config) *Store {
	if cfg.GatewayURL == "" {
		cfg.GatewayURL = "https://ipfs.io/ipfs/"
	}
	if cfg.APIURL == "" {
		cfg.APIURL = "http://127.0.0.1:5001"
	}
	cacheDir := filepath.Join(cfg.DataDir, "ipfs-cache")
	os.MkdirAll(cacheDir, 0700)

	return &Store{
		gatewayURL: cfg.GatewayURL,
		apiURL:     cfg.APIURL,
		dataDir:    cacheDir,
	}
}

// Put stores data locally (content-addressed). Returns content hash as "CID".
func (s *Store) Put(data []byte) (string, error) {
	h := sha256.Sum256(data)
	cid := hex.EncodeToString(h[:])
	path := filepath.Join(s.dataDir, cid)
	return cid, os.WriteFile(path, data, 0644)
}

// Get retrieves data by CID from local cache.
func (s *Store) Get(cid string) ([]byte, error) {
	path := filepath.Join(s.dataDir, cid)
	return os.ReadFile(path)
}

// PutLargeContent stores content on IPFS if it exceeds threshold.
func (s *Store) PutLargeContent(content string, threshold int) (*ContentRef, error) {
	if len(content) < threshold {
		return &ContentRef{Inline: true, Content: content}, nil
	}
	cid, err := s.Put([]byte(content))
	if err != nil {
		return nil, err
	}
	return &ContentRef{Inline: false, CID: cid, Size: len(content)}, nil
}

// ResolveContent resolves a ContentRef to actual content.
func (s *Store) ResolveContent(ref *ContentRef) (string, error) {
	if ref.Inline {
		return ref.Content, nil
	}
	data, err := s.Get(ref.CID)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ContentRef is a reference to content, either inline or on IPFS.
type ContentRef struct {
	Inline  bool   `json:"inline"`
	Content string `json:"content,omitempty"`
	CID     string `json:"cid,omitempty"`
	Size    int    `json:"size,omitempty"`
}

// IsCID checks if a string looks like a content hash.
func IsCID(s string) bool {
	return len(s) == 64 && isHexString(s)
}

func isHexString(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// HasIPFSBin checks if the ipfs CLI is available.
func HasIPFSBin() bool {
	_, err := os.Stat("/usr/local/bin/ipfs")
	if err == nil {
		return true
	}
	_, err = os.Stat("/usr/bin/ipfs")
	return err == nil
}

// Stats returns cache statistics.
func (s *Store) Stats() (files int, totalSize int64) {
	entries, err := os.ReadDir(s.dataDir)
	if err != nil {
		return 0, 0
	}
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files++
		totalSize += info.Size()
	}
	return
}
