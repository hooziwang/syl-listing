package output

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type brokenReader struct{}

func (brokenReader) Read(p []byte) (int, error) { return 0, io.EOF }

func TestEnsureDir(t *testing.T) {
	d := t.TempDir()
	target := filepath.Join(d, "a", "b")
	if err := EnsureDir(target); err != nil {
		t.Fatalf("EnsureDir error: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("stat error: %v", err)
	}
	if err := EnsureDir(""); err == nil {
		t.Fatalf("expected empty dir error")
	}
}

func TestNextPairAndNextEN(t *testing.T) {
	d := t.TempDir()
	rand := bytes.NewReader(bytes.Repeat([]byte{1}, 64))
	id, en, cn, err := NextPair(d, 8, rand)
	if err != nil {
		t.Fatalf("NextPair error: %v", err)
	}
	if id == "" || en == "" || cn == "" {
		t.Fatalf("invalid outputs")
	}
	if err := os.WriteFile(en, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cn, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	id2, en2, err := NextEN(d, 8, bytes.NewReader(bytes.Repeat([]byte{2}, 64)))
	if err != nil {
		t.Fatalf("NextEN error: %v", err)
	}
	if id2 == "" || en2 == "" {
		t.Fatalf("invalid NextEN output")
	}
}

func TestRandomIDReadError(t *testing.T) {
	_, err := randomID(8, brokenReader{})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, io.EOF) {
		t.Fatalf("unexpected err: %v", err)
	}
}

type constantReader struct{ b byte }

func (c constantReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = c.b
	}
	return len(p), nil
}

func TestNextPairCollisionExhaustion(t *testing.T) {
	d := t.TempDir()
	// constantReader will always produce the same ID; pre-create conflicting files so 1000 retries all collide.
	randSrc := constantReader{b: 1}
	id, en, cn, err := NextPair(d, 8, randSrc)
	if err != nil {
		t.Fatalf("initial NextPair should succeed: %v", err)
	}
	if id == "" {
		t.Fatalf("id should not be empty")
	}
	if err := os.WriteFile(en, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cn, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, _, err = NextPair(d, 8, randSrc)
	if err == nil {
		t.Fatalf("expected collision exhaustion error")
	}
}

func TestNextENCollisionExhaustion(t *testing.T) {
	d := t.TempDir()
	randSrc := constantReader{b: 2}
	id, en, err := NextEN(d, 8, randSrc)
	if err != nil {
		t.Fatalf("initial NextEN should succeed: %v", err)
	}
	if id == "" {
		t.Fatalf("id should not be empty")
	}
	if err := os.WriteFile(en, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err = NextEN(d, 8, randSrc)
	if err == nil {
		t.Fatalf("expected NextEN collision exhaustion error")
	}
}
