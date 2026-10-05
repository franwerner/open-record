package e2e

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/franwerner/open-record/internal/qmd"
)

// Every scenario here scopes into the `api` component of e2e/project, whose
// records are real and checked in — so a literal match is a genuine one, not
// a fixture built just for this test.
const searchTerm = "rate limit"

// gatewayTitle and quotasTitle are the real titles of two records under
// decisions/api/security/rate-limits, used to steer the Jev double's scores
// without inventing fixtures of our own.
const (
	gatewayTitle = "Rate limiting is applied at the gateway"
	quotasTitle  = "Whether a key can have its own quota"
)

// TestSearchServesAndStoresEndToEnd drives a full, successful search against
// the real fixture and checks every externally visible consequence: the
// output shape, the stored search file on disk, that it is a snapshot (a
// later edit to the file does not change it), and that it is invisible to
// git and to the other commands' own scope.
func TestSearchServesAndStoresEndToEnd(t *testing.T) {
	repo := project(t)
	env := mergeEnv(stubQmd(t, workingQmdWithEmbeddings(repo, "api")), stubJev(t, map[string]float64{
		gatewayTitle: 0.9,
		quotasTitle:  0.1,
	}, 0.05))

	code, stdout, stderr := runWith(t, repo, env, "search", "--for", "decisions/api",
		"--literal", searchTerm, "--semantic", "rate limiting policy",
		"--context", "adding a per-user rate limit")
	if code != exitOK {
		t.Fatalf("search failed: %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	report := decode[searchOutput](t, stdout)
	if report.ID == "" || report.Model == "" {
		t.Fatalf("missing id or model: %+v", report)
	}

	var gateway *searchRecord
	for index := range report.Records {
		if report.Records[index].Path == "decisions/api/security/rate-limits/at-the-gateway.md" {
			gateway = &report.Records[index]
		}
	}
	if gateway == nil {
		t.Fatalf("the record both the literal term and the top score favour is missing: %+v", report.Records)
	}
	if gateway.Literal == nil || gateway.Literal.Hits == 0 {
		t.Errorf("the literal hit was not recorded: %+v", gateway)
	}

	// The stored search exists, under the id the output named.
	storedPath := filepath.Join(repo, ".openrecord", ".searches", report.ID+".json")
	raw, err := os.ReadFile(storedPath)
	if err != nil {
		t.Fatalf("no stored search at %s: %v", storedPath, err)
	}
	var stored struct {
		Served []struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		} `json:"served"`
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("stored search is not valid JSON: %v", err)
	}
	var servedGateway *struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	for index := range stored.Served {
		if stored.Served[index].Path == "decisions/api/security/rate-limits/at-the-gateway.md" {
			servedGateway = &stored.Served[index]
		}
	}
	if servedGateway == nil || servedGateway.Content == "" {
		t.Fatalf("the served record's full content was not stored: %+v", stored)
	}

	// A snapshot: editing the file after the search must not change what was
	// stored.
	live := filepath.Join(repo, ".openrecord", "decisions", "api", "security", "rate-limits", "at-the-gateway.md")
	if err := os.WriteFile(live, []byte(servedGateway.Content+"\nedited after the search\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reread, err := os.ReadFile(storedPath)
	if err != nil {
		t.Fatal(err)
	}
	var restored struct {
		Served []struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		} `json:"served"`
	}
	if err := json.Unmarshal(reread, &restored); err != nil {
		t.Fatal(err)
	}
	var rerereadGateway string
	for _, entry := range restored.Served {
		if entry.Path == "decisions/api/security/rate-limits/at-the-gateway.md" {
			rerereadGateway = entry.Content
		}
	}
	if rerereadGateway != servedGateway.Content || strings.Contains(rerereadGateway, "edited after the search") {
		t.Error("the stored snapshot changed after the live file was edited")
	}

	// git never sees it: --porcelain omits an ignored path by default, the
	// same view a plain `git status` gives a person.
	gitInit(t, repo)
	status := gitStatusPorcelain(t, repo)
	if strings.Contains(status, ".searches") {
		t.Errorf("git status lists the stored search: %s", status)
	}
}

// TestSearchIsInvisibleToOtherCommands checks that validate, map and grep
// produce byte-identical output whether or not a stored search exists.
func TestSearchIsInvisibleToOtherCommands(t *testing.T) {
	repo := project(t)

	_, mapBefore, _ := run(t, repo, "map", "--for", "decisions/api")
	_, grepBefore, _ := run(t, repo, "grep", "rate limit", "--for", "decisions/api")
	_, validateBefore, _ := run(t, repo, "validate", "--for", "decisions/api")

	env := mergeEnv(stubQmd(t, workingQmdWithEmbeddings(repo, "api")), stubJev(t, nil, 0.5))
	code, _, stderr := runWith(t, repo, env, "search", "--for", "decisions/api",
		"--literal", searchTerm, "--semantic", "rate limiting", "--context", "a test")
	if code != exitOK {
		t.Fatalf("search failed: %s", stderr)
	}

	_, mapAfter, _ := run(t, repo, "map", "--for", "decisions/api")
	_, grepAfter, _ := run(t, repo, "grep", "rate limit", "--for", "decisions/api")
	_, validateAfter, _ := run(t, repo, "validate", "--for", "decisions/api")

	if mapBefore != mapAfter {
		t.Error("map's output changed after a stored search was created")
	}
	if grepBefore != grepAfter {
		t.Error("grep's output changed after a stored search was created")
	}
	if validateBefore != validateAfter {
		t.Error("validate's output changed after a stored search was created")
	}
}

// TestSearchFailsWhenKeyIsMissing checks the precondition order end to end:
// with no key set, the Jev double sees no request at all.
func TestSearchFailsWhenKeyIsMissing(t *testing.T) {
	repo := project(t)
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	env := mergeEnv(stubQmd(t, workingQmdWithEmbeddings(repo, "api")),
		[]string{"OPENROUTER_API_KEY=", "OPENRECORD_JEV_ENDPOINT=" + server.URL})

	code, stdout, stderr := runWith(t, repo, env, "search", "--for", "decisions/api",
		"--literal", searchTerm, "--semantic", "rate limiting", "--context", "a test")
	if code == exitOK {
		t.Fatal("search ran with no key set")
	}
	if stdout != "" {
		t.Errorf("a failing search wrote to stdout: %s", stdout)
	}
	if called {
		t.Error("a request reached the Jev endpoint despite no key being set")
	}
	result := decode[finding](t, stderr)
	if result.Code != "jev-unavailable" {
		t.Errorf("code = %q, want jev-unavailable", result.Code)
	}
}

// TestSearchFailsWhenJevErrors checks that a failing Jev request fails the
// whole search, with nothing stored.
func TestSearchFailsWhenJevErrors(t *testing.T) {
	repo := project(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"code":"internal","message":"boom"}}`))
	}))
	defer server.Close()

	env := mergeEnv(stubQmd(t, workingQmdWithEmbeddings(repo, "api")),
		[]string{"OPENROUTER_API_KEY=a-key", "OPENRECORD_JEV_ENDPOINT=" + server.URL})

	code, stdout, stderr := runWith(t, repo, env, "search", "--for", "decisions/api",
		"--literal", searchTerm, "--semantic", "rate limiting", "--context", "a test")
	if code == exitOK {
		t.Fatal("search succeeded despite every Jev request failing")
	}
	if stdout != "" {
		t.Errorf("a failing search wrote to stdout: %s", stdout)
	}
	result := decode[finding](t, stderr)
	if result.Code != "jev-unavailable" || !strings.Contains(result.Message, "jev status") {
		t.Errorf("stderr = %+v, want jev-unavailable naming `openrecord jev status`", result)
	}

	entries, _ := os.ReadDir(filepath.Join(repo, ".openrecord", ".searches"))
	if len(entries) != 0 {
		t.Errorf("a search was stored despite failing: %+v", entries)
	}
}

// TestSearchQueryCarriesNoRerankAndJevSeesOnlyTitlesAndDescriptions is the
// runtime check both halves of the published disclosure rest on: qmd never
// reranks or expands at query time, and Jev receives the context plus only
// titles and descriptions — never a record's body.
func TestSearchQueryCarriesNoRerankAndJevSeesOnlyTitlesAndDescriptions(t *testing.T) {
	repo := project(t)
	argvFile := filepath.Join(t.TempDir(), "argv")

	qmdEnv := stubQmd(t, `
case "$1 $2" in
  "collection list") echo '{"schemaVersion":1,"collections":[{"name":"`+qmd.DecisionsCollection(filepath.Base(repo), "api")+`"}]}'; exit 0 ;;
esac
case "$1" in
  --version|-v) echo "qmd 2.8.3-mate.7"; exit 0 ;;
  status) echo "QMD Status"; exit 0 ;;
  capabilities) echo '{"schemaVersion":1,"embed":{"available":true}}'; exit 0 ;;
  query) echo "$@" > "$ARGV_FILE"; echo '[]'; exit 0 ;;
  *) exit 0 ;;
esac
`)
	qmdEnv = append(qmdEnv, "ARGV_FILE="+argvFile)

	var capturedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		capturedBody += string(raw)
		var body struct {
			Questions map[string]any `json:"questions"`
		}
		_ = json.Unmarshal(raw, &body)
		answers := make(map[string]map[string]float64, len(body.Questions))
		for name := range body.Questions {
			answers[name] = map[string]float64{"noul": 0.1}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"answers": answers})
	}))
	defer server.Close()
	jevEnv := []string{"OPENROUTER_API_KEY=a-key", "OPENRECORD_JEV_ENDPOINT=" + server.URL}

	context := "adding a per-user rate limit to the signup endpoint"
	code, stdout, stderr := runWith(t, repo, mergeEnv(qmdEnv, jevEnv), "search", "--for", "decisions/api",
		"--literal", searchTerm, "--semantic", "rate limiting", "--context", context)
	if code != exitOK {
		t.Fatalf("search failed: %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	_ = decode[searchOutput](t, stdout)

	argv, err := os.ReadFile(argvFile)
	if err != nil {
		t.Fatalf("the query stub never captured its own argv: %v", err)
	}
	if !strings.Contains(string(argv), "--no-rerank") {
		t.Errorf("the invoked qmd argv does not carry --no-rerank: %q", argv)
	}
	for _, forbidden := range []string{"--rerank", "--expand", "--expansion", "--rerank-model", "--generate"} {
		if strings.Contains(string(argv), forbidden) {
			t.Errorf("the invoked qmd argv carries %q: %q", forbidden, argv)
		}
	}

	if !strings.Contains(capturedBody, context) {
		t.Errorf("the Jev request does not carry the context: %s", capturedBody)
	}
	if strings.Contains(capturedBody, "Rejected:") || strings.Contains(capturedBody, "## Context") {
		t.Errorf("the Jev request carries record body text, which must never be sent: %s", capturedBody)
	}
}

// TestSearchReviewFlowEndToEnd exercises review open/mark/status against a
// search this test itself creates, and jev status against the same double.
func TestSearchReviewFlowEndToEnd(t *testing.T) {
	repo := project(t)
	env := mergeEnv(stubQmd(t, workingQmdWithEmbeddings(repo, "api")), stubJev(t, map[string]float64{
		gatewayTitle: 0.9,
	}, 0.5))

	stdout := mustRunWith(t, repo, env, "search", "--for", "decisions/api",
		"--literal", searchTerm, "--semantic", "rate limiting", "--context", "a test")
	report := decode[searchOutput](t, stdout)
	if len(report.Records) == 0 {
		t.Fatal("nothing was served")
	}

	for _, record := range report.Records {
		mustRunWith(t, repo, env, "review", "open", report.ID, record.Path)
	}
	for _, record := range report.Records {
		mustRunWith(t, repo, env, "review", "mark", report.ID, record.Path, "--verdict", "unrelated")
	}
	code, statusOut, statusErr := runWith(t, repo, env, "review", "status", report.ID)
	if code != exitOK {
		t.Fatalf("review status failed once every served record was marked: %s", statusErr)
	}
	if !strings.Contains(statusOut, "completed_at") {
		t.Errorf("status does not report completed_at: %s", statusOut)
	}
}

// TestSearchFailsWhenProjectIsNotIndexed is scenario (e): qmd is on the PATH
// and usable, but no collection is registered for this search's scope — D8
// fails the search before a Jev request is ever built.
func TestSearchFailsWhenProjectIsNotIndexed(t *testing.T) {
	repo := project(t)
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	// No components passed: only the specs collection is reported registered,
	// never project-decisions-api, which is all this --for scope needs.
	env := mergeEnv(stubQmd(t, workingQmdWithEmbeddings(repo)),
		[]string{"OPENROUTER_API_KEY=a-key", "OPENRECORD_JEV_ENDPOINT=" + server.URL})

	code, stdout, stderr := runWith(t, repo, env, "search", "--for", "decisions/api",
		"--literal", searchTerm, "--semantic", "rate limiting", "--context", "a test")
	if code == exitOK {
		t.Fatal("search ran against a project with no qmd index for its scope")
	}
	if stdout != "" {
		t.Errorf("a failing search wrote to stdout: %s", stdout)
	}
	if called {
		t.Error("a request reached the Jev endpoint despite there being no index")
	}
	result := decode[finding](t, stderr)
	if result.Code != "qmd-not-indexed" || !strings.Contains(result.Message, "qmd index") {
		t.Errorf("stderr = %+v, want qmd-not-indexed naming `openrecord qmd index`", result)
	}
}

// TestSearchReadsTheKeyFromDotenvWhenProcessEnvLacksIt is RS-1's "Key from
// .env" scenario, end to end: the key lives only in `.openrecord/.env`, the
// process environment genuinely never carries it (stubQmd's own environment
// is PATH and HOME only — no inherited key to fall back on), and the Jev
// double must still receive it in the Authorization header.
func TestSearchReadsTheKeyFromDotenvWhenProcessEnvLacksIt(t *testing.T) {
	repo := project(t)

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
	defer server.Close()

	if err := os.WriteFile(filepath.Join(repo, ".openrecord", ".env"),
		[]byte("OPENROUTER_API_KEY=dotenv-only-key\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Deliberately no OPENROUTER_API_KEY entry here: stubQmd's own
	// environment carries only PATH and HOME, so the subprocess the binary
	// runs in has no key at all except through the .env file just written.
	env := mergeEnv(stubQmd(t, workingQmdWithEmbeddings(repo, "api")),
		[]string{"OPENRECORD_JEV_ENDPOINT=" + server.URL})

	code, stdout, stderr := runWith(t, repo, env, "search", "--for", "decisions/api",
		"--literal", searchTerm, "--semantic", "rate limiting", "--context", "a test")
	if code != exitOK {
		t.Fatalf("search failed: %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	report := decode[searchOutput](t, stdout)
	if len(report.Records) == 0 {
		t.Fatal("nothing was served")
	}
	if gotAuth != "Bearer dotenv-only-key" {
		t.Errorf("authorization = %q, want the key declared only in .openrecord/.env", gotAuth)
	}
}

// TestJevStatusEndToEnd exercises `jev status` against a real double, for
// both a missing key and a healthy one.
func TestJevStatusEndToEnd(t *testing.T) {
	repo := project(t)

	code, stdout, _ := runWith(t, repo, []string{"OPENROUTER_API_KEY="}, "jev", "status")
	if code == exitOK {
		t.Error("jev status with no key exited 0")
	}
	var noKey struct {
		APIKey bool `json:"api_key"`
	}
	if err := json.Unmarshal([]byte(stdout), &noKey); err != nil || noKey.APIKey {
		t.Errorf("jev status with no key = %s", stdout)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Questions map[string]any `json:"questions"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		answers := make(map[string]map[string]float64, len(body.Questions))
		for name := range body.Questions {
			answers[name] = map[string]float64{"noul": 0.1}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"answers": answers})
	}))
	defer server.Close()

	code, stdout, stderr := runWith(t, repo, []string{"OPENROUTER_API_KEY=a-key", "OPENRECORD_JEV_ENDPOINT=" + server.URL}, "jev", "status")
	if code != exitOK {
		t.Fatalf("jev status healthy failed: %s", stderr)
	}
	var healthy struct {
		APIKey    bool `json:"api_key"`
		Reachable bool `json:"reachable"`
	}
	if err := json.Unmarshal([]byte(stdout), &healthy); err != nil || !healthy.APIKey || !healthy.Reachable {
		t.Errorf("jev status healthy = %s", stdout)
	}
}

// mustRunWith is mustRun with a replaced environment.
func mustRunWith(t *testing.T, repo string, env []string, args ...string) string {
	t.Helper()
	code, stdout, stderr := runWith(t, repo, env, args...)
	if code != exitOK {
		t.Fatalf("`openrecord %s` exited %d\nstdout: %s\nstderr: %s", strings.Join(args, " "), code, stdout, stderr)
	}
	return stdout
}

// gitInit makes repo a git repository, the way a real project is one.
func gitInit(t *testing.T, repo string) {
	t.Helper()
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = repo
	if err := cmd.Run(); err != nil {
		t.Skipf("git is not available to run this check: %v", err)
	}
}

// gitStatusPorcelain reports git's own view of the repository — ignored
// paths are omitted by default, the same as a plain `git status` a person
// would run.
func gitStatusPorcelain(t *testing.T, repo string) string {
	t.Helper()
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git status failed: %v\n%s", err, out)
	}
	return string(out)
}
