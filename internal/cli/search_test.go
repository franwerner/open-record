package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/qmd"
	"github.com/franwerner/open-record/internal/store"
)

// noSemanticHits is a qmd stub that answers every subcommand search needs —
// --version and status for Probe, collection list for the D8 "is this
// project indexed" check (reporting every collection repo's declared
// components resolve to as already registered, so that check never trips
// here), capabilities for the embedding check, and an empty query result —
// so every scenario here that is not itself about the semantic pass, or
// about D8 itself, gets no contribution from it.
func noSemanticHits(repo string, components ...string) string {
	project := filepath.Base(repo)
	names := qmd.Collections(project, components)
	entries := make([]string, len(names))
	for index, name := range names {
		entries[index] = fmt.Sprintf(`{"name":%q}`, name)
	}
	collections := fmt.Sprintf(`{"schemaVersion":1,"collections":[%s]}`, strings.Join(entries, ","))
	return fmt.Sprintf(`
case "$1 $2" in
  "collection list") echo '%s'; exit 0 ;;
esac
case "$1" in
  --version|-v) echo "qmd 2.8.3-mate.7"; exit 0 ;;
  status) echo "QMD Status"; exit 0 ;;
  capabilities) echo '{"schemaVersion":1,"embed":{"available":true}}'; exit 0 ;;
  query) echo '[]'; exit 0 ;;
  *) exit 0 ;;
esac
`, collections)
}

// writeSearchRecord writes a record with an empty title and description, so
// Jev's instruction falls back to the record's path as its title — which is
// what stubJev below keys its scores on.
func writeSearchRecord(t *testing.T, repo, path, body string) {
	t.Helper()
	writeRecord(t, filepath.Join(repo, store.Root, filepath.FromSlash(path)),
		"---\ntitle: \"\"\ndescription: \"\"\nstatus: accepted\n---\n\n"+body+"\n")
}

// writeSearchRecordWithDescription is writeSearchRecord with a caller-chosen
// description, used to inflate one item's token estimate past the default
// chunk budget without touching its title (and so without touching the path
// stubJev keys its scores on).
func writeSearchRecordWithDescription(t *testing.T, repo, path, description, body string) {
	t.Helper()
	writeRecord(t, filepath.Join(repo, store.Root, filepath.FromSlash(path)),
		"---\ntitle: \"\"\ndescription: "+description+"\nstatus: accepted\n---\n\n"+body+"\n")
}

// jevTitlePattern recovers the path Jev was asked about from the instruction
// text runSearch sends: with an empty title, jev.instructionFor falls back to
// item.Path, which is exactly what is quoted after `Record `.
var jevTitlePattern = regexp.MustCompile(`Record "([^"]*)":`)

// stubJevEndpoint starts a local Jev double, scoring each question by the
// path its instruction names — looked up in scores, or fallback when absent
// — and points the client seam and the key at it for the running test.
func stubJevEndpoint(t *testing.T, scores map[string]float64, fallback float64) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Questions map[string]struct {
				Instructions string `json:"instructions"`
			} `json:"questions"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		answers := make(map[string]map[string]float64, len(body.Questions))
		for name, question := range body.Questions {
			score := fallback
			if match := jevTitlePattern.FindStringSubmatch(question.Instructions); match != nil {
				if s, ok := scores[match[1]]; ok {
					score = s
				}
			}
			answers[name] = map[string]float64{"noul": score}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"answers": answers})
	}))
	t.Cleanup(server.Close)
	t.Setenv("OPENROUTER_API_KEY", "a-key")
	t.Setenv("OPENRECORD_JEV_ENDPOINT", server.URL)
}

// searchArgs builds a complete, valid invocation, letting one test override a
// single flag's value (or drop it entirely with an empty override map that
// still names the flag) — so a flag-validation test only has to say what is
// different about it.
func searchArgs(overrides map[string]string, extra ...string) []string {
	base := map[string]string{"--for": "decisions/api", "--literal": "term", "--semantic": "a query", "--context": "the task"}
	for flag, value := range overrides {
		if value == "" {
			delete(base, flag)
			continue
		}
		base[flag] = value
	}
	args := []string{"search"}
	for flag, value := range base {
		args = append(args, flag, value)
	}
	args = append(args, extra...)
	return args
}

func TestSearchRequiresEveryFlag(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")

	for _, flag := range []string{"--for", "--literal", "--semantic", "--context"} {
		t.Run(flag, func(t *testing.T) {
			code, _, stderr := runIn(t, repo, searchArgs(map[string]string{flag: ""})...)
			if code == exitOK {
				t.Fatalf("search without %s was accepted", flag)
			}
			result := decode[finding.Finding](t, stderr)
			if result.Code != finding.CodeUsage {
				t.Errorf("code = %q, want %q", result.Code, finding.CodeUsage)
			}
			name := strings.TrimPrefix(flag, "--")
			if !strings.Contains(result.Message, name) {
				t.Errorf("message does not name %s: %q", flag, result.Message)
			}
		})
	}
}

func TestSearchRejectsMoreThanTenLiteralTerms(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")

	args := []string{"search", "--for", "decisions/api", "--semantic", "q", "--context", "c"}
	for i := 0; i < 11; i++ {
		args = append(args, "--literal", strings.Repeat("x", i+1))
	}
	code, _, stderr := runIn(t, repo, args...)
	if code == exitOK {
		t.Fatal("11 --literal values were accepted")
	}
	result := decode[finding.Finding](t, stderr)
	if result.Code != finding.CodeUsage || !strings.Contains(result.Message, "10") {
		t.Errorf("message = %q, want a usage failure naming the limit of 10", result.Message)
	}
}

func TestSearchRejectsAPositionalArgument(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")

	// The positional argument must come after every flag: flag.Parse stops
	// consuming flags at the first non-flag token, so putting it first would
	// leave --for (etc.) unparsed instead of exercising this check.
	code, _, stderr := runIn(t, repo, searchArgs(nil, "extra")...)
	if code == exitOK {
		t.Fatal("a positional argument was accepted")
	}
	result := decode[finding.Finding](t, stderr)
	if result.Code != finding.CodeUsage {
		t.Errorf("code = %q, want %q", result.Code, finding.CodeUsage)
	}
}

func TestSearchLiteralCommaStaysInOneTerm(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")
	writeSearchRecord(t, repo, "decisions/api/security/a.md", "this record says a,b together")
	writeSearchRecord(t, repo, "decisions/api/security/b.md", "this record only says a alone")
	stubQmd(t, noSemanticHits(repo, "api"))
	stubJevEndpoint(t, nil, 0.0)

	report := decode[searchOutput](t, mustRun(t, repo,
		searchArgs(map[string]string{"--literal": "a,b"})...))

	var a, b *searchRecord
	for index := range report.Records {
		switch report.Records[index].Path {
		case "decisions/api/security/a.md":
			a = &report.Records[index]
		case "decisions/api/security/b.md":
			b = &report.Records[index]
		}
	}
	if a == nil || a.Literal == nil {
		t.Fatalf("the record containing the literal \"a,b\" was not matched: %+v", report.Records)
	}
	if b != nil && b.Literal != nil {
		t.Errorf("\"a,b\" was split into separate terms — the comma leaked into a second match: %+v", b)
	}
}

func TestSearchServingFloorOfThreeAndDiscardedOrder(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"} {
		writeSearchRecord(t, repo, "decisions/api/security/"+name+".md", "nothing special here")
	}
	stubQmd(t, noSemanticHits(repo, "api"))
	// Every record ties at the same low score: the floor must still serve
	// exactly 3, breaking the tie by path ascending.
	stubJevEndpoint(t, nil, 0.05)

	report := decode[searchOutput](t, mustRun(t, repo,
		searchArgs(map[string]string{"--literal": "unmatched-term"})...))

	if len(report.Records) != 3 {
		t.Fatalf("records = %d, want 3 (the floor): %+v", len(report.Records), report.Records)
	}
	wantServed := []string{"decisions/api/security/a.md", "decisions/api/security/b.md", "decisions/api/security/c.md"}
	for index, path := range wantServed {
		if report.Records[index].Path != path {
			t.Errorf("records[%d] = %s, want %s (ties break by path ascending)", index, report.Records[index].Path, path)
		}
	}
	if len(report.Discarded) != 7 {
		t.Fatalf("discarded = %d, want 7", len(report.Discarded))
	}
	if !sort.StringsAreSorted(report.Discarded) {
		t.Errorf("discarded is not in path order: %v", report.Discarded)
	}
}

// RS-1 "Key from .env": with the key declared only in `.openrecord/.env`
// and the process environment genuinely unset (unsetenv, not merely
// emptied), the key must still reach Jev's Authorization header — proving
// FromEnv is read through env.Vars.Getenv (the merged dotenv+process view),
// never through the process environment directly.
func TestSearchReadsTheKeyFromDotenvWhenProcessEnvIsUnset(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")
	writeSearchRecord(t, repo, "decisions/api/security/a.md", "nothing special here")
	stubQmd(t, noSemanticHits(repo, "api"))

	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		var body struct {
			Questions map[string]any `json:"questions"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		answers := make(map[string]map[string]float64, len(body.Questions))
		for name := range body.Questions {
			answers[name] = map[string]float64{"noul": 0.5}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"answers": answers})
	}))
	t.Cleanup(server.Close)

	unsetenv(t, "OPENROUTER_API_KEY")
	t.Setenv("OPENRECORD_JEV_ENDPOINT", server.URL)
	if err := os.MkdirAll(filepath.Join(repo, store.Root), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, store.Root, ".env"), []byte("OPENROUTER_API_KEY=dotenv-only-key\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	report := decode[searchOutput](t, mustRun(t, repo,
		searchArgs(map[string]string{"--literal": "unmatched-term"})...))
	if len(report.Records) == 0 {
		t.Fatal("nothing was served")
	}
	if gotAuth != "Bearer dotenv-only-key" {
		t.Errorf("authorization = %q, want the key declared only in .openrecord/.env", gotAuth)
	}
}

func TestSearchServingEveryRecordAboveThreshold(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		writeSearchRecord(t, repo, "decisions/api/security/"+name+".md", "nothing special")
	}
	stubQmd(t, noSemanticHits(repo, "api"))
	stubJevEndpoint(t, nil, 0.5)

	report := decode[searchOutput](t, mustRun(t, repo,
		searchArgs(map[string]string{"--literal": "unmatched-term"})...))

	if len(report.Records) != 5 {
		t.Fatalf("records = %d, want 5 (all above the threshold)", len(report.Records))
	}
	if len(report.Discarded) != 0 {
		t.Fatalf("discarded = %v, want none", report.Discarded)
	}
}

func TestSearchServingSmallScope(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")
	writeSearchRecord(t, repo, "decisions/api/security/a.md", "x")
	writeSearchRecord(t, repo, "decisions/api/security/b.md", "x")
	stubQmd(t, noSemanticHits(repo, "api"))
	stubJevEndpoint(t, nil, 0.0)

	report := decode[searchOutput](t, mustRun(t, repo,
		searchArgs(map[string]string{"--literal": "unmatched-term"})...))

	if len(report.Records) != 2 {
		t.Fatalf("records = %d, want 2 (the whole scope)", len(report.Records))
	}
	if len(report.Discarded) != 0 {
		t.Errorf("discarded = %v, want empty", report.Discarded)
	}
}

func TestSearchServingALowScoreLiteralHitStillServes(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")
	writeSearchRecord(t, repo, "decisions/api/security/high1.md", "x")
	writeSearchRecord(t, repo, "decisions/api/security/high2.md", "x")
	writeSearchRecord(t, repo, "decisions/api/security/high3.md", "x")
	writeSearchRecord(t, repo, "decisions/api/security/low.md", "this one mentions needle")
	writeSearchRecord(t, repo, "decisions/api/security/discarded.md", "x")
	stubQmd(t, noSemanticHits(repo, "api"))
	stubJevEndpoint(t, map[string]float64{
		"decisions/api/security/high1.md":     0.9,
		"decisions/api/security/high2.md":     0.8,
		"decisions/api/security/high3.md":     0.7,
		"decisions/api/security/low.md":       0.01,
		"decisions/api/security/discarded.md": 0.02,
	}, 0.0)

	report := decode[searchOutput](t, mustRun(t, repo,
		searchArgs(map[string]string{"--literal": "needle"})...))

	if len(report.Records) != 4 {
		t.Fatalf("records = %d, want 4 (the top 3 plus the literal hit): %+v", len(report.Records), report.Records)
	}
	last := report.Records[len(report.Records)-1]
	if last.Path != "decisions/api/security/low.md" || last.Literal == nil {
		t.Errorf("the low-scoring literal hit is not last and carrying its match: %+v", last)
	}
	if len(report.Discarded) != 1 || report.Discarded[0] != "decisions/api/security/discarded.md" {
		t.Errorf("discarded = %v, want exactly the record with neither a hit nor a top score", report.Discarded)
	}
}

func TestSearchOmitRemovesAndCounts(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")
	writeSearchRecord(t, repo, "decisions/api/security/a.md", "x")
	writeSearchRecord(t, repo, "decisions/api/security/b.md", "x")
	stubQmd(t, noSemanticHits(repo, "api"))
	stubJevEndpoint(t, nil, 0.0)

	report := decode[searchOutput](t, mustRun(t, repo,
		searchArgs(map[string]string{"--literal": "unmatched-term"}, "--omit", "decisions/api/security/a.md")...))

	if report.Omitted != 1 {
		t.Errorf("omitted = %d, want 1", report.Omitted)
	}
	for _, record := range report.Records {
		if record.Path == "decisions/api/security/a.md" {
			t.Error("the omitted record was still served")
		}
	}
	for _, path := range report.Discarded {
		if path == "decisions/api/security/a.md" {
			t.Error("the omitted record was still discarded")
		}
	}
	if len(report.Records)+len(report.Discarded) != 1 {
		t.Errorf("want exactly one remaining scoped record, got %d served and %d discarded", len(report.Records), len(report.Discarded))
	}
}

func TestSearchOmitOfAnUnscopedPathCountsZero(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")
	writeSearchRecord(t, repo, "decisions/api/security/a.md", "x")
	stubQmd(t, noSemanticHits(repo, "api"))
	stubJevEndpoint(t, nil, 0.0)

	report := decode[searchOutput](t, mustRun(t, repo,
		searchArgs(map[string]string{"--literal": "unmatched-term"}, "--omit", "decisions/api/security/never-found.md")...))

	if report.Omitted != 0 {
		t.Errorf("omitted = %d, want 0", report.Omitted)
	}
}

// A literal term matching only a level's INDEX.md must serve nothing on that
// basis: groups are never in scope, so the index is never scanned at all.
func TestSearchGroupOnlyLiteralHitServesNothing(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API",
		"--description", "A very-distinctive-zzqux word only the index carries.")
	writeSearchRecord(t, repo, "decisions/api/security/a.md", "nothing related here")
	stubQmd(t, noSemanticHits(repo, "api"))
	stubJevEndpoint(t, nil, 0.0)

	report := decode[searchOutput](t, mustRun(t, repo,
		searchArgs(map[string]string{"--literal": "zzqux"})...))

	for _, record := range report.Records {
		if record.Literal != nil {
			t.Errorf("a literal hit confined to an INDEX.md leaked into a served record: %+v", record)
		}
	}
}

// The precondition order is flags, then the key, then qmd: a missing key must
// fail before anything reaches the Jev endpoint, regardless of whether qmd is
// even on the PATH.
func TestSearchChecksTheKeyBeforeAnyJevRequest(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")
	writeSearchRecord(t, repo, "decisions/api/security/a.md", "x")

	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("OPENRECORD_JEV_ENDPOINT", server.URL)

	code, _, stderr := runIn(t, repo, searchArgs(nil)...)
	if code == exitOK {
		t.Fatal("search with no key was accepted")
	}
	if called {
		t.Error("a request reached the Jev endpoint despite no key being set")
	}
	result := decode[finding.Finding](t, stderr)
	if result.Code != finding.CodeJevUnavailable {
		t.Errorf("code = %q, want %q", result.Code, finding.CodeJevUnavailable)
	}
}

// D8 (RS-2): with a usable qmd but nothing registered yet — the state before
// `openrecord qmd index` has ever run — search must fail naming that
// command, and the Jev double must never be contacted: the check runs
// before capabilities or query, well before any item is ever scored.
func TestSearchFailsWhenNothingIsIndexedYet(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")
	writeSearchRecord(t, repo, "decisions/api/security/a.md", "x")

	stubQmd(t, `
case "$1 $2" in
  "collection list") echo '{"schemaVersion":1,"collections":[]}'; exit 0 ;;
esac
case "$1" in
  --version|-v) echo "qmd 2.8.3-mate.7"; exit 0 ;;
  status) echo "QMD Status"; exit 0 ;;
  *) exit 0 ;;
esac
`)

	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()
	t.Setenv("OPENROUTER_API_KEY", "a-key")
	t.Setenv("OPENRECORD_JEV_ENDPOINT", server.URL)

	code, _, stderr := runIn(t, repo, searchArgs(nil)...)
	if code == exitOK {
		t.Fatal("search with no project index was accepted")
	}
	if called {
		t.Error("a request reached the Jev endpoint despite no project index being built")
	}
	result := decode[finding.Finding](t, stderr)
	if result.Code != finding.CodeQmdNotIndexed {
		t.Errorf("code = %q, want %q", result.Code, finding.CodeQmdNotIndexed)
	}
	if !strings.Contains(result.Message, "openrecord qmd index") {
		t.Errorf("message does not name `openrecord qmd index`: %q", result.Message)
	}
}

// The output must not depend on how many Jev chunks the scope was split
// across. Each item's description is inflated well past the default token
// budget, forcing more than one request; the merged scores must still rank
// and serve exactly as a single request would.
func TestSearchMergesScoresAcrossMultipleJevChunks(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")

	big := strings.Repeat("x ", 15000)
	paths := []string{
		"decisions/api/security/a.md", "decisions/api/security/b.md", "decisions/api/security/c.md",
		"decisions/api/security/d.md", "decisions/api/security/e.md",
	}
	for _, p := range paths {
		writeSearchRecordWithDescription(t, repo, p, big, "irrelevant body")
	}
	stubQmd(t, noSemanticHits(repo, "api"))
	stubJevEndpoint(t, map[string]float64{
		paths[0]: 0.9, paths[1]: 0.1, paths[2]: 0.95, paths[3]: 0.05, paths[4]: 0.3,
	}, 0.0)

	report := decode[searchOutput](t, mustRun(t, repo,
		searchArgs(map[string]string{"--literal": "unmatched-term"})...))

	// c (.95), a (.9) and e (.3) score above the threshold; the floor adds
	// nothing beyond them since they are already the top 3.
	want := []string{paths[2], paths[0], paths[4]}
	if len(report.Records) != len(want) {
		t.Fatalf("records = %+v, want paths %v", report.Records, want)
	}
	for index, p := range want {
		if report.Records[index].Path != p {
			t.Errorf("records[%d] = %s, want %s — scores must survive the chunk split correctly ordered", index, report.Records[index].Path, p)
		}
	}
}

// Unlike TestSearchMergesScoresAcrossMultipleJevChunks, where every
// description is large enough to force a split but still fits the budget on
// its own, this one description alone exceeds the default 32000-token
// budget: no split can help, so chunkPlan must fail before any Jev request
// is sent, and runSearch must map that into a usage failure naming "context
// too long" rather than jev-unavailable — distinguishing "the question is
// too big" from "Jev could not be reached".
func TestSearchFailsWithUsageWhenADescriptionExceedsTheBudget(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")

	// ~180000 runes / 3.5 ≈ 51400 estimated tokens for this one description
	// alone, well past the 32000-token default budget with margin for the
	// fixed wording instructionFor wraps around it.
	tooBig := strings.Repeat("x ", 90000)
	writeSearchRecordWithDescription(t, repo, "decisions/api/security/a.md", tooBig, "irrelevant body")
	stubQmd(t, noSemanticHits(repo, "api"))
	// Never actually contacted: chunkPlan refuses before Score sends
	// anything. Still pointed at a loopback double, not the real default
	// endpoint, so a regression that did send a request could not reach out.
	stubJevEndpoint(t, nil, 0)

	code, _, stderr := runIn(t, repo, searchArgs(nil)...)
	if code == exitOK {
		t.Fatal("search with a description past the token budget was accepted")
	}
	result := decode[finding.Finding](t, stderr)
	if result.Code != finding.CodeUsage {
		t.Errorf("code = %q, want %q (not %q — this is a bad request, not an unreachable Jev)", result.Code, finding.CodeUsage, finding.CodeJevUnavailable)
	}
	if !strings.Contains(result.Message, "context too long") {
		t.Errorf("message = %q, want it to name %q", result.Message, "context too long")
	}
}
