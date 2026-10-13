package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStampHashIsStableForTheSameContent(t *testing.T) {
	set := Components{Components: []Component{{ID: "api", Paths: []string{"src/api"}}}}
	first, err := StampHash(set)
	if err != nil {
		t.Fatal(err)
	}
	second, err := StampHash(set)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Errorf("StampHash is not stable: %s vs %s", first, second)
	}
	if len(first) != 64 {
		t.Errorf("StampHash = %q, want 64 hex characters", first)
	}
}

func TestStampHashChangesWithContent(t *testing.T) {
	a := Components{Components: []Component{{ID: "api", Paths: []string{"src/api"}}}}
	b := Components{Components: []Component{{ID: "api", Paths: []string{"src/api", "src/extra"}}}}
	hashA, err := StampHash(a)
	if err != nil {
		t.Fatal(err)
	}
	hashB, err := StampHash(b)
	if err != nil {
		t.Fatal(err)
	}
	if hashA == hashB {
		t.Error("StampHash did not change when content changed")
	}
}

// TestStampHashExcludesItsOwnField proves the hash is computed with the
// field itself cleared: carrying any prior hash value must never change what
// gets stamped, the same way body-hash never hashes itself.
func TestStampHashExcludesItsOwnField(t *testing.T) {
	a := Components{Components: []Component{{ID: "api", Paths: []string{"src/api"}}}, Hash: ""}
	b := Components{Components: []Component{{ID: "api", Paths: []string{"src/api"}}}, Hash: "whatever-was-there-before"}
	hashA, err := StampHash(a)
	if err != nil {
		t.Fatal(err)
	}
	hashB, err := StampHash(b)
	if err != nil {
		t.Fatal(err)
	}
	if hashA != hashB {
		t.Error("a prior hash value changed what StampHash computed; the field was not excluded")
	}
}

func TestSaveStampsHashAsTheLastKey(t *testing.T) {
	repo := t.TempDir()
	set := Components{Components: []Component{{ID: "api", Paths: []string{"src/api"}}}}
	if err := set.Save(repo); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(ComponentsFile)))
	if err != nil {
		t.Fatal(err)
	}
	trimmed := strings.TrimRight(string(raw), "\n")
	if !strings.HasSuffix(trimmed, "}") {
		t.Fatalf("file does not end with a closing brace: %q", trimmed)
	}
	lines := strings.Split(trimmed, "\n")
	// The line right before the closing brace is the last field written.
	last := strings.TrimSpace(lines[len(lines)-2])
	if !strings.HasPrefix(last, `"hash":`) {
		t.Errorf("hash is not the last key written: last field line = %q", last)
	}
}

func TestVerifyHashAcceptsWhatSaveWrote(t *testing.T) {
	repo := t.TempDir()
	set := Components{Components: []Component{{ID: "api", Paths: []string{"src/api"}}}}
	if err := set.Save(repo); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadComponents(repo)
	if err != nil {
		t.Fatal(err)
	}
	if findings := VerifyComponents(repo, reloaded); len(findings) != 0 {
		t.Errorf("VerifyHash rejected what Save wrote: %+v", findings)
	}
}

func TestVerifyHashReportsAFieldAddedByHand(t *testing.T) {
	// Parsing drops a key the type does not declare, so the recomputed hash
	// still matches; only comparing the bytes on disk catches it.
	repo := t.TempDir()
	set := Components{Components: []Component{{ID: "api", Paths: []string{"src/api"}}}}
	if err := set.Save(repo); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(repo, filepath.FromSlash(ComponentsFile))
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(raw), `"id": "api",`, `"id": "api",
      "owner": "mallory",`, 1)
	if err := os.WriteFile(file, []byte(tampered), 0o644); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadComponents(repo)
	if err != nil {
		t.Fatal(err)
	}
	findings := VerifyComponents(repo, reloaded)
	if len(findings) != 1 || findings[0].Code != "json-hash-mismatch" {
		t.Fatalf("VerifyComponents(unknown field added) = %+v, want exactly one json-hash-mismatch", findings)
	}
}

func TestVerifyHashReportsAMissingHash(t *testing.T) {
	set := Components{Components: []Component{{ID: "api", Paths: []string{"src/api"}}}}
	findings := VerifyHash(ComponentsFile, nil, set)
	if len(findings) != 1 || findings[0].Code != "json-hash-missing" {
		t.Fatalf("VerifyHash(no hash) = %+v, want exactly one json-hash-missing", findings)
	}
}

func TestVerifyHashReportsAMismatch(t *testing.T) {
	set := Components{Components: []Component{{ID: "api", Paths: []string{"src/api"}}}}
	hash, err := StampHash(set)
	if err != nil {
		t.Fatal(err)
	}
	set.Hash = hash
	// Tamper with the content without recomputing the hash — the same shape
	// a hand-edit that adds a path produces.
	set.Components = append(set.Components, Component{ID: "ui", Paths: []string{"src/ui"}})
	findings := VerifyHash(ComponentsFile, nil, set)
	if len(findings) != 1 || findings[0].Code != "json-hash-mismatch" {
		t.Fatalf("VerifyHash(tampered) = %+v, want exactly one json-hash-mismatch", findings)
	}
}
