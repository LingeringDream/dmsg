package ipfs

import (
	"testing"
)

func TestPutAndGet(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(Config{DataDir: dir})

	data := []byte("hello ipfs world")
	cid, err := s.Put(data)
	if err != nil {
		t.Fatal(err)
	}
	if cid == "" {
		t.Fatal("expected non-empty CID")
	}

	got, err := s.Get(cid)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Fatal("retrieved data mismatch")
	}
}

func TestContentRef(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(Config{DataDir: dir})

	// Small content → inline
	ref, err := s.PutLargeContent("short", 100)
	if err != nil {
		t.Fatal(err)
	}
	if !ref.Inline {
		t.Fatal("small content should be inline")
	}

	content, err := s.ResolveContent(ref)
	if err != nil {
		t.Fatal(err)
	}
	if content != "short" {
		t.Fatal("inline content mismatch")
	}

	// Large content → stored
	ref2, err := s.PutLargeContent("this is a very long message that exceeds the threshold for inline storage", 20)
	if err != nil {
		t.Fatal(err)
	}
	if ref2.Inline {
		t.Fatal("large content should not be inline")
	}
	if ref2.CID == "" {
		t.Fatal("expected non-empty CID for stored content")
	}

	content2, err := s.ResolveContent(ref2)
	if err != nil {
		t.Fatal(err)
	}
	if len(content2) < 50 {
		t.Fatal("resolved content too short")
	}
}

func TestIsCID(t *testing.T) {
	if IsCID("hello") {
		t.Fatal("short string is not a CID")
	}

	// 64 hex chars = SHA256 hash
	validHash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if !IsCID(validHash) {
		t.Fatal("valid hex hash should be CID")
	}
}

func TestStats(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(Config{DataDir: dir})

	s.Put([]byte("data1"))
	s.Put([]byte("data2"))

	files, size := s.Stats()
	if files != 2 {
		t.Fatalf("expected 2 files, got %d", files)
	}
	if size == 0 {
		t.Fatal("expected non-zero size")
	}
}

func TestHasIPFSBin(t *testing.T) {
	// Just test it doesn't panic
	_ = HasIPFSBin()
}

func TestContentRefFields(t *testing.T) {
	ref := &ContentRef{Inline: false, CID: "abc123", Size: 1000}
	if ref.Inline {
		t.Fatal("expected non-inline ref")
	}
	if ref.CID != "abc123" {
		t.Fatal("CID mismatch")
	}
	if ref.Size != 1000 {
		t.Fatal("Size mismatch")
	}
}
