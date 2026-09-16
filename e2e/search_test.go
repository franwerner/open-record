package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every scenario here scopes into the `api` component of e2e/project, whose
// records are real and checked in — so a literal match is a genuine one, not
// a fixture built just for this test.
const searchTerm = "rate limit"

// TestSearchTranslatesQmdHitsIntoStoreCoordinates drives the built binary with
// a stub qmd whose semantic pass finds a real record by a term that is not in
// its text at all — isolating a semantic-origin hit from anything the literal
// pass could have found on its own — and checks it comes back as a plain
// store coordinate, never the qmd:// URL form.
func TestSearchTranslatesQmdHitsIntoStoreCoordinates(t *testing.T) {
	repo := project(t)
	env := stubQmd(t, `
case "$1" in
  capabilities) echo '{"schemaVersion":1,"embed":{"available":true}}'; exit 0 ;;
  query) echo '[{"file":"qmd://project-decisions-api/security/rate-limits/per-key-quotas.md","line":4,"snippet":"a per-key ceiling for one integration partner"}]'; exit 0 ;;
  *) exit 0 ;;
esac
`)

	code, stdout, stderr := runWith(t, repo, env, "search", "an unrelated phrase found by meaning alone", "--for", "decisions/api")
	if code != exitOK {
		t.Fatalf("search failed: %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	report := decode[searchReport](t, stdout)

	var translated *searchMatch
	for index := range report.Matches {
		if report.Matches[index].Path == "decisions/api/security/rate-limits/per-key-quotas.md" {
			translated = &report.Matches[index]
		}
	}
	if translated == nil {
		t.Fatalf("the qmd:// hit was not translated into a store coordinate: %+v", report.Matches)
	}
	if translated.Hits != 1 {
		t.Errorf("a semantic-origin entry must carry the hits:1 placeholder, got %+v", translated)
	}
	for _, match := range report.Matches {
		if strings.Contains(match.Path, "qmd://") {
			t.Errorf("a qmd:// URL leaked into the output: %+v", match)
		}
	}
}

// TestSearchFiltersToTheRequestedSubtree scopes below the component root and
// checks that a semantic hit from a sibling concern — which the query
// necessarily returned, since qmd can only be scoped to the whole collection —
// is dropped, while one actually inside the requested subtree survives.
func TestSearchFiltersToTheRequestedSubtree(t *testing.T) {
	repo := project(t)
	env := stubQmd(t, `
case "$1" in
  capabilities) echo '{"schemaVersion":1,"embed":{"available":true}}'; exit 0 ;;
  query) echo '[{"file":"qmd://project-decisions-api/security/rate-limits/per-key-quotas.md","line":1,"snippet":"inside the requested subtree"},{"file":"qmd://project-decisions-api/runtime/error-translation.md","line":1,"snippet":"outside the requested subtree"}]'; exit 0 ;;
  *) exit 0 ;;
esac
`)

	code, stdout, stderr := runWith(t, repo, env, "search", "an unrelated phrase found by meaning alone", "--for", "decisions/api/security")
	if code != exitOK {
		t.Fatalf("search failed: %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	report := decode[searchReport](t, stdout)

	for _, match := range report.Matches {
		if !strings.HasPrefix(match.Path, "decisions/api/security") {
			t.Errorf("a match escaped decisions/api/security: %+v", match)
		}
	}
	found := false
	for _, match := range report.Matches {
		if match.Path == "decisions/api/security/rate-limits/per-key-quotas.md" {
			found = true
		}
	}
	if !found {
		t.Error("the hit inside the requested subtree was dropped along with the one outside it")
	}
}

// TestSearchMergePrefersTheLiteralEntry uses a term that is genuinely in a
// real record's text, and a stub that also claims to have found the same
// path by meaning with fabricated line/snippet/hits — checking the merged
// entry is the literal pass's, never the semantic placeholder.
func TestSearchMergePrefersTheLiteralEntry(t *testing.T) {
	repo := project(t)
	env := stubQmd(t, `
case "$1" in
  capabilities) echo '{"schemaVersion":1,"embed":{"available":true}}'; exit 0 ;;
  query) echo '[{"file":"qmd://project-decisions-api/security/rate-limits/at-the-gateway.md","line":999,"snippet":"a fabricated snippet nothing in the real record says"}]'; exit 0 ;;
  *) exit 0 ;;
esac
`)

	code, stdout, stderr := runWith(t, repo, env, "search", searchTerm, "--for", "decisions/api")
	if code != exitOK {
		t.Fatalf("search failed: %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	report := decode[searchReport](t, stdout)

	var gateway *searchMatch
	for index := range report.Matches {
		if report.Matches[index].Path == "decisions/api/security/rate-limits/at-the-gateway.md" {
			gateway = &report.Matches[index]
		}
	}
	if gateway == nil {
		t.Fatalf("the record both passes found is missing: %+v", report.Matches)
	}
	if gateway.Line != 2 || gateway.Hits != 2 || gateway.Text != "title: Rate limiting is applied at the gateway" {
		t.Errorf("the shared path did not keep the literal entry, got %+v", gateway)
	}

	seen := map[string]int{}
	for _, match := range report.Matches {
		seen[match.Path]++
	}
	for path, count := range seen {
		if count > 1 {
			t.Errorf("%s appears %d times; matches must be deduplicated by path", path, count)
		}
	}
}

// TestSearchSemanticOutcomes is the full state table this design's whole
// three-value field exists for: what qmd answers about itself, and what the
// query itself does, join into the one `semantic` value a caller reads.
func TestSearchSemanticOutcomes(t *testing.T) {
	cases := []struct {
		name   string
		script string
		want   string
	}{
		{
			name: "the embedding model is reachable",
			script: `
case "$1" in
  capabilities) echo '{"schemaVersion":1,"embed":{"available":true}}'; exit 0 ;;
  query) echo '[]'; exit 0 ;;
  *) exit 0 ;;
esac
`,
			want: "used",
		},
		{
			name: "the embedding model is reported unreachable",
			script: `
case "$1" in
  capabilities) echo '{"schemaVersion":1,"embed":{"available":false}}'; exit 0 ;;
  query) echo '[]'; exit 0 ;;
  *) exit 0 ;;
esac
`,
			want: "lexical-only",
		},
		{
			// This is the shape of an older qmd that predates the subcommand:
			// no `capabilities` arm at all, everything else answers cleanly.
			name:   "no capabilities subcommand at all",
			script: workingQmd,
			want:   "lexical-only",
		},
		{
			// A qmd that recognises `capabilities` as a name but refuses it.
			// The query still has to succeed here for the case to say
			// anything distinct from the next one: it is the capabilities
			// read failing on its own that this degrades, never the query.
			name: "capabilities exits non-zero",
			script: `
case "$1" in
  capabilities) exit 1 ;;
  query) echo '[]'; exit 0 ;;
  *) exit 0 ;;
esac
`,
			want: "lexical-only",
		},
		{
			name: "the query itself exits non-zero",
			script: `
case "$1" in
  capabilities) echo '{"schemaVersion":1,"embed":{"available":true}}'; exit 0 ;;
  query) exit 1 ;;
  *) exit 0 ;;
esac
`,
			want: "unavailable",
		},
		{
			name: "the query returns unparseable output",
			script: `
case "$1" in
  capabilities) echo '{"schemaVersion":1,"embed":{"available":true}}'; exit 0 ;;
  query) echo "not json"; exit 0 ;;
  *) exit 0 ;;
esac
`,
			want: "unavailable",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			repo := project(t)
			env := stubQmd(t, testCase.script)
			code, stdout, stderr := runWith(t, repo, env, "search", "anything", "--for", "decisions/api")
			if code != exitOK {
				t.Fatalf("search failed: %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
			}
			report := decode[searchReport](t, stdout)
			if report.Semantic != testCase.want {
				t.Errorf("semantic = %q, want %q", report.Semantic, testCase.want)
			}
		})
	}
}

// TestSearchQueryCarriesNoRerankAndNoExpansionStep is the runtime check the
// whole docs/cli.md reconciliation rests on: "no model runs at query time"
// beyond the embedding itself. qmd.Query hardcodes --no-rerank in the args it
// builds (qmd.go:265), but nothing inspected what the invoked qmd actually
// received. The stub captures its own argv to a file — the only way to see
// what crossed the process boundary, rather than trusting what the caller
// believes it sent.
func TestSearchQueryCarriesNoRerankAndNoExpansionStep(t *testing.T) {
	repo := project(t)
	argvFile := filepath.Join(t.TempDir(), "argv")

	env := stubQmd(t, `
case "$1" in
  capabilities) echo '{"schemaVersion":1,"embed":{"available":true}}'; exit 0 ;;
  query) echo "$@" > "$ARGV_FILE"; echo '[]'; exit 0 ;;
  *) exit 0 ;;
esac
`)
	env = append(env, "ARGV_FILE="+argvFile)

	code, stdout, stderr := runWith(t, repo, env, "search", "anything", "--for", "decisions/api")
	if code != exitOK {
		t.Fatalf("search failed: %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	_ = decode[searchReport](t, stdout)

	raw, err := os.ReadFile(argvFile)
	if err != nil {
		t.Fatalf("the query stub never captured its own argv — the query subprocess was not invoked as expected: %v", err)
	}
	argv := string(raw)

	if !strings.Contains(argv, "--no-rerank") {
		t.Errorf("the invoked qmd argv does not carry --no-rerank: %q", argv)
	}
	// Anything that would trigger a rerank or query-expansion step at query
	// time contradicts the claim entirely — a flag re-enabling rerank, or a
	// distinct expansion pass. --no-rerank alone is what the code sends; any
	// of these appearing means a model beyond the embedding itself ran.
	for _, forbidden := range []string{"--rerank", "--expand", "--expansion", "--rerank-model", "--generate"} {
		if strings.Contains(argv, forbidden) {
			t.Errorf("the invoked qmd argv carries %q, which the no-model-at-query-time claim forbids: %q", forbidden, argv)
		}
	}
}
