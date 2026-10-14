// JSON integrity (task 02),
// proved against the built binary and the realistic fixture: components.json
// carries a `hash` stamped by `component add`/`component remove`, `validate`
// reports a missing or mismatching one, and a writer refuses to rewrite a
// file it cannot trust rather than silently laundering a hand-edit. The
// mechanism itself (internal/store/integrity.go) is unit-tested next to
// where it lives; this only proves the behaviour against the built binary,
// the way anchors_test.go does for anchors.
package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// componentsJSON is the shape read back off disk, declared locally for the
// same reason the other harness shapes are: a test that shared the
// producer's struct could not notice a field being renamed.
type componentsJSON struct {
	Version    string `json:"version"`
	Components []struct {
		ID    string   `json:"id"`
		Paths []string `json:"paths"`
	} `json:"components"`
	Hash string `json:"hash"`
}

func componentsPath(repo string) string {
	return filepath.Join(repo, ".openrecord", "components.json")
}

func readComponents(t *testing.T, repo string) ([]byte, componentsJSON) {
	t.Helper()
	raw, err := os.ReadFile(componentsPath(repo))
	if err != nil {
		t.Fatal(err)
	}
	var decoded componentsJSON
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("components.json is not valid JSON: %v\n%s", err, raw)
	}
	return raw, decoded
}

// plantAPath hand-edits components.json to add an extra path to its first
// component, the same shape a hand-edit adding a path nobody declared with
// `component add` produces: still perfectly valid JSON, content changed, the
// stored hash left stale. The one deliberate hand-edit-the-store exception,
// same pattern anchors_test.go uses for its planted duplicate anchor: a
// broken file planted directly on disk, never produced through openrecord
// itself.
func plantAPath(t *testing.T, raw []byte) []byte {
	t.Helper()
	tampered := strings.Replace(string(raw), `"paths": [`, "\"paths\": [\n      \"src/planted-by-hand\",", 1)
	if tampered == string(raw) {
		t.Fatal("could not plant an extra path into components.json for this test")
	}
	return []byte(tampered)
}

func hasCode(findings []finding, code string) bool {
	for _, item := range findings {
		if item.Code == code {
			return true
		}
	}
	return false
}

func TestIntegrity(t *testing.T) {
	t.Run("component add and remove stamp the hash last, and it changes on remove", func(t *testing.T) {
		repo := project(t)

		mustRun(t, repo, "component", "add", "extra", "--path", "src/extra",
			"--title", "Extra", "--description", "A fixture-only surface for this test.")

		raw, decoded := readComponents(t, repo)
		if decoded.Hash == "" {
			t.Fatal("component add did not stamp a hash")
		}
		trimmed := strings.TrimRight(string(raw), "\n")
		lines := strings.Split(trimmed, "\n")
		last := strings.TrimSpace(lines[len(lines)-2])
		if !strings.HasPrefix(last, `"hash":`) {
			t.Errorf("hash is not the last key: %q", last)
		}
		afterAdd := decoded.Hash

		if _, stdout, _ := run(t, repo, "validate"); strings.Contains(stdout, "json-hash") {
			t.Errorf("validate reported a json-hash finding right after component add: %s", stdout)
		}

		mustRun(t, repo, "component", "remove", "extra")
		_, afterRemove := readComponents(t, repo)
		if afterRemove.Hash == "" {
			t.Fatal("component remove did not recompute a hash")
		}
		if afterRemove.Hash == afterAdd {
			t.Error("component remove left the hash unchanged even though the content changed")
		}
		if code, stdout, _ := run(t, repo, "validate"); code != exitOK {
			t.Fatalf("validate failed after component remove: %s", stdout)
		}
	})

	t.Run("validate reports a missing hash", func(t *testing.T) {
		repo := project(t)
		full := componentsPath(repo)
		raw, decoded := readComponents(t, repo)
		if decoded.Hash == "" {
			t.Fatal("fixture components.json has no hash to remove; re-run e2e/seed.sh")
		}

		withoutHash := strings.Replace(string(raw),
			",\n  \"hash\": \""+decoded.Hash+"\"\n", "\n", 1)
		if withoutHash == string(raw) {
			t.Fatal("could not strip the hash field from components.json for this test")
		}
		if err := os.WriteFile(full, []byte(withoutHash), 0o644); err != nil {
			t.Fatal(err)
		}

		code, stdout, _ := run(t, repo, "validate")
		if code == exitOK {
			t.Fatalf("validate did not fail on a hash-less components.json: %s", stdout)
		}
		report := decode[validateReport](t, stdout)
		if !hasCode(report.Findings, "json-hash-missing") {
			t.Errorf("validate findings = %v, want json-hash-missing", findingCodes(report.Findings))
		}
	})

	t.Run("validate reports a hand-planted path as a mismatch", func(t *testing.T) {
		repo := project(t)
		raw, _ := readComponents(t, repo)
		if err := os.WriteFile(componentsPath(repo), plantAPath(t, raw), 0o644); err != nil {
			t.Fatal(err)
		}

		code, stdout, _ := run(t, repo, "validate")
		if code == exitOK {
			t.Fatalf("validate did not report the hand-planted path: %s", stdout)
		}
		report := decode[validateReport](t, stdout)
		if !hasCode(report.Findings, "json-hash-mismatch") {
			t.Errorf("validate findings = %v, want json-hash-mismatch", findingCodes(report.Findings))
		}
	})

	t.Run("component add and remove refuse a tampered components.json, file untouched", func(t *testing.T) {
		repo := project(t)
		full := componentsPath(repo)
		raw, _ := readComponents(t, repo)
		tampered := plantAPath(t, raw)
		if err := os.WriteFile(full, tampered, 0o644); err != nil {
			t.Fatal(err)
		}

		t.Run("add", func(t *testing.T) {
			code, _, stderr := run(t, repo, "component", "add", "shouldnotland",
				"--path", "src/shouldnotland", "--title", "t", "--description", "d")
			if code == exitOK {
				t.Fatal("component add succeeded against a tampered components.json")
			}
			if !strings.Contains(stderr, "json-hash-mismatch") {
				t.Errorf("component add did not report json-hash-mismatch: %s", stderr)
			}
			after, err := os.ReadFile(full)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(tampered) {
				t.Error("component add changed the tampered file's bytes")
			}
		})

		t.Run("remove", func(t *testing.T) {
			code, _, stderr := run(t, repo, "component", "remove", "api")
			if code == exitOK {
				t.Fatal("component remove succeeded against a tampered components.json")
			}
			if !strings.Contains(stderr, "json-hash-mismatch") {
				t.Errorf("component remove did not report json-hash-mismatch: %s", stderr)
			}
			after, err := os.ReadFile(full)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(tampered) {
				t.Error("component remove changed the tampered file's bytes")
			}
		})
	})

	t.Run("record edit refuses a hand-edited body, file untouched", func(t *testing.T) {
		repo := project(t)
		target := "decisions/api/runtime/error-translation.md"
		full := filepath.Join(repo, ".openrecord", filepath.FromSlash(target))

		before, err := os.ReadFile(full)
		if err != nil {
			t.Fatal(err)
		}
		tampered := strings.Replace(string(before), "\n\n", "\n\nHand-edited outside the tool.\n\n", 1)
		if tampered == string(before) {
			t.Fatal("could not tamper with the record body for this test")
		}
		if err := os.WriteFile(full, []byte(tampered), 0o644); err != nil {
			t.Fatal(err)
		}

		code, stdout, _ := run(t, repo, "record", "edit", target, "--title", "new title")
		if code == exitOK {
			t.Fatalf("record edit succeeded against a hand-edited body: %s", stdout)
		}
		if !strings.Contains(stdout, "body-hash-mismatch") {
			t.Errorf("record edit did not report body-hash-mismatch: %s", stdout)
		}
		after, err := os.ReadFile(full)
		if err != nil {
			t.Fatal(err)
		}
		if string(after) != tampered {
			t.Error("record edit changed the hand-edited file's bytes")
		}
	})

	t.Run("map and grep still work against a tampered components.json; only validate and writers reject", func(t *testing.T) {
		repo := project(t)
		raw, _ := readComponents(t, repo)
		if err := os.WriteFile(componentsPath(repo), plantAPath(t, raw), 0o644); err != nil {
			t.Fatal(err)
		}

		if code, stdout, stderr := run(t, repo, "map", "--for", "decisions"); code != exitOK {
			t.Fatalf("map failed against a tampered components.json: stdout=%s stderr=%s", stdout, stderr)
		}
		if code, stdout, stderr := run(t, repo, "grep", "gateway", "--for", "decisions/api"); code != exitOK {
			t.Fatalf("grep failed against a tampered components.json: stdout=%s stderr=%s", stdout, stderr)
		}
		if code, stdout, _ := run(t, repo, "validate"); code == exitOK {
			t.Errorf("validate accepted a tampered components.json: %s", stdout)
		}
	})

	t.Run("restoring the original bytes clears the tamper for validate and writers", func(t *testing.T) {
		repo := project(t)
		full := componentsPath(repo)
		original, _ := readComponents(t, repo)
		if err := os.WriteFile(full, plantAPath(t, original), 0o644); err != nil {
			t.Fatal(err)
		}
		if code, stdout, _ := run(t, repo, "validate"); code == exitOK {
			t.Fatalf("validate did not notice the tamper before restoring: %s", stdout)
		}

		// Simulate `git checkout -- .openrecord/components.json`.
		if err := os.WriteFile(full, original, 0o644); err != nil {
			t.Fatal(err)
		}

		if code, stdout, _ := run(t, repo, "validate"); code != exitOK {
			t.Fatalf("validate still failed after restoring the original bytes: %s", stdout)
		}
		mustRun(t, repo, "component", "add", "restored", "--path", "src/restored",
			"--title", "Restored", "--description", "Proves writers work again after a restore.")
	})
}
