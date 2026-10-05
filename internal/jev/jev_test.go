package jev

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestEstimateTokensRoundsUp(t *testing.T) {
	cases := []struct {
		text string
		want int
	}{
		{"", 0},
		{"abc", 1},     // 3 runes / 3.5 -> ceil(0.857) = 1
		{"abcdefg", 2}, // 7 runes / 3.5 = 2 exactly
		{"abcdefgh", 3},
	}
	for _, c := range cases {
		if got := estimateTokens(c.text); got != c.want {
			t.Errorf("estimateTokens(%q) = %d, want %d", c.text, got, c.want)
		}
	}
}

func TestBuildQuestionsNamesByPathAndResolvesCollisions(t *testing.T) {
	items := []Item{
		{Path: "decisions/api/rate-limit.md", Title: "Rate limit", Description: "d"},
		{Path: "decisions/api/rate_limit.md", Title: "Rate limit 2", Description: "d"},
		{Path: "decisions/api/rate.limit.md", Title: "Rate limit 3", Description: "d"},
	}
	questions := buildQuestions(items)
	if len(questions) != 3 {
		t.Fatalf("got %d questions, want 3", len(questions))
	}
	want := []string{"decisions_api_rate_limit_md", "decisions_api_rate_limit_md_2", "decisions_api_rate_limit_md_3"}
	for index, q := range questions {
		if q.Name != want[index] {
			t.Errorf("question[%d].Name = %q, want %q", index, q.Name, want[index])
		}
	}
}

func TestInstructionForFallsBackToPathAndTrimsTrailingStop(t *testing.T) {
	withTitle := instructionFor(Item{Path: "p.md", Title: "A title", Description: "A description."})
	if !strings.Contains(withTitle, `"A title"`) {
		t.Errorf("instruction does not carry the title: %q", withTitle)
	}
	if strings.Contains(withTitle, "description..") {
		t.Errorf("trailing stop was not trimmed: %q", withTitle)
	}

	withoutTitle := instructionFor(Item{Path: "p.md", Description: "d"})
	if !strings.Contains(withoutTitle, `"p.md"`) {
		t.Errorf("instruction does not fall back to the path: %q", withoutTitle)
	}
}

func TestChunkPlanSplitsOnlyWhenNeeded(t *testing.T) {
	questions := []question{
		{Name: "a", Instructions: "x"},
		{Name: "b", Instructions: "y"},
	}
	chunks, err := chunkPlan(questions, 0, defaultBudget)
	if err != nil {
		t.Fatalf("chunkPlan: %v", err)
	}
	if len(chunks) != 1 || len(chunks[0]) != 2 {
		t.Fatalf("got %d chunks, want everything in one chunk: %v", len(chunks), chunks)
	}
}

func TestChunkPlanSplitsIntoNearEqualContiguousGroups(t *testing.T) {
	// Each question costs roughly estimateTokens(name)+estimateTokens(instr)+34.
	// A tiny budget forces several chunks; the plan must keep every chunk
	// under the unit and every question exactly once, in order.
	var questions []question
	for i := 0; i < 7; i++ {
		questions = append(questions, question{Name: fmt.Sprintf("q%d", i), Instructions: "short"})
	}
	unit := questionTokens(questions[0])*3 - 1 // forces more than 3 per chunk to fail
	chunks, err := chunkPlan(questions, defaultBudget-unit, defaultBudget)
	if err != nil {
		t.Fatalf("chunkPlan: %v", err)
	}
	total := 0
	seen := map[string]bool{}
	for _, chunk := range chunks {
		if len(chunk) == 0 {
			t.Fatal("an empty chunk was produced")
		}
		sum := 0
		for _, q := range chunk {
			sum += questionTokens(q)
			if seen[q.Name] {
				t.Fatalf("%s scored in more than one chunk", q.Name)
			}
			seen[q.Name] = true
		}
		if sum > unit {
			t.Errorf("chunk exceeds the unit: %d > %d", sum, unit)
		}
		total += len(chunk)
	}
	if total != len(questions) {
		t.Fatalf("chunks hold %d questions total, want %d", total, len(questions))
	}
}

func TestChunkPlanFailsWhenNoSplitCanHelp(t *testing.T) {
	questions := []question{{Name: "a", Instructions: strings.Repeat("x", 1000)}}
	_, err := chunkPlan(questions, 0, 1) // budget of 1 token, far below one question
	if err == nil {
		t.Fatal("expected a usage failure when the budget cannot fit the largest question")
	}
	if !strings.Contains(err.Error(), "context too long") {
		t.Errorf("error = %v, want it to name \"context too long\"", err)
	}
}

// stubServer builds an httptest.Server whose handler decides the response per
// request, and a Client pointed at it.
func stubServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, Client) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server, Client{Endpoint: server.URL, Key: "test-key", HTTP: server.Client()}
}

// decodeRequest reads the question names a request carried, for a handler
// that wants to answer each one.
func decodeRequest(t *testing.T, r *http.Request) map[string]struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions"`
} {
	t.Helper()
	var body struct {
		Questions map[string]struct {
			Type         string `json:"type"`
			Instructions string `json:"instructions"`
		} `json:"questions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Fatalf("decoding request body: %v", err)
	}
	return body.Questions
}

func writeAnswers(w http.ResponseWriter, names []string, score func(string) float64) {
	answers := make(map[string]map[string]float64, len(names))
	for _, name := range names {
		answers[name] = map[string]float64{"noul": score(name)}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"answers": answers})
}

func TestScoreOneChunkAndManyChunksProduceIdenticalScores(t *testing.T) {
	items := make([]Item, 0, 9)
	for i := 0; i < 9; i++ {
		items = append(items, Item{
			Path:        fmt.Sprintf("decisions/api/r%d.md", i),
			Title:       fmt.Sprintf("Record %d", i),
			Description: "d",
		})
	}

	fixedScore := func(name string) float64 {
		// A score derived from the name so it is deterministic and distinct
		// per record, regardless of which chunk it is answered in.
		sum := 0
		for _, r := range name {
			sum += int(r)
		}
		return float64(sum%100) / 100
	}

	server, client := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
		questions := decodeRequest(t, r)
		names := make([]string, 0, len(questions))
		for name := range questions {
			names = append(names, name)
		}
		writeAnswers(w, names, fixedScore)
	})
	_ = server

	oneChunk, err := client.Score(context.Background(), "ctx", items)
	if err != nil {
		t.Fatalf("one-chunk Score: %v", err)
	}

	client.Budget = 200 // small enough to force several chunks
	manyChunks, err := client.Score(context.Background(), "ctx", items)
	if err != nil {
		t.Fatalf("many-chunk Score: %v", err)
	}

	if len(oneChunk) != len(items) || len(manyChunks) != len(items) {
		t.Fatalf("got %d and %d scores, want %d", len(oneChunk), len(manyChunks), len(items))
	}
	for _, item := range items {
		if oneChunk[item.Path] != manyChunks[item.Path] {
			t.Errorf("%s: one-chunk score %v != many-chunk score %v", item.Path, oneChunk[item.Path], manyChunks[item.Path])
		}
	}
}

func TestScoreSplitsOnceOnMaxTokensExceededAndFailsOnASecondRefusal(t *testing.T) {
	var calls int32
	server, client := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
		questions := decodeRequest(t, r)
		n := atomic.AddInt32(&calls, 1)
		// The first call (the full chunk of 2) is refused. The two halves
		// that follow (each a single question) must succeed.
		if n == 1 {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{"code": "max_tokens_exceeded", "message": "max_tokens_exceeded: too many"},
			})
			return
		}
		names := make([]string, 0, len(questions))
		for name := range questions {
			names = append(names, name)
		}
		writeAnswers(w, names, func(string) float64 { return 0.5 })
	})
	_ = server

	items := []Item{
		{Path: "a.md", Title: "A", Description: "d"},
		{Path: "b.md", Title: "B", Description: "d"},
	}
	scores, err := client.Score(context.Background(), "ctx", items)
	if err != nil {
		t.Fatalf("Score with a recoverable split: %v", err)
	}
	if len(scores) != 2 {
		t.Fatalf("got %d scores, want 2", len(scores))
	}

	// Now every call refuses with max_tokens_exceeded — the second refusal
	// (on an already-split half) must fail rather than split again.
	atomic.StoreInt32(&calls, 0)
	server2, client2 := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"code": "max_tokens_exceeded", "message": "max_tokens_exceeded: too many"},
		})
	})
	_ = server2
	if _, err := client2.Score(context.Background(), "ctx", items); err == nil {
		t.Fatal("expected a failure when the split halves also refuse")
	}
}

func TestScoreRetriesOnceOnTransientFailures(t *testing.T) {
	t.Run("429 then success", func(t *testing.T) {
		var calls int32
		server, client := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
			if atomic.AddInt32(&calls, 1) == 1 {
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			questions := decodeRequest(t, r)
			names := make([]string, 0, len(questions))
			for name := range questions {
				names = append(names, name)
			}
			writeAnswers(w, names, func(string) float64 { return 0.1 })
		})
		_ = server
		start := time.Now()
		scores, err := client.Score(context.Background(), "ctx", []Item{{Path: "a.md", Title: "A", Description: "d"}})
		if err != nil {
			t.Fatalf("Score: %v", err)
		}
		if elapsed := time.Since(start); elapsed < time.Second {
			t.Errorf("retry did not wait before trying again: %s", elapsed)
		}
		if len(scores) != 1 {
			t.Fatalf("got %d scores, want 1", len(scores))
		}
	})

	t.Run("500 twice fails", func(t *testing.T) {
		server, client := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
		_ = server
		if _, err := client.Score(context.Background(), "ctx", []Item{{Path: "a.md", Title: "A", Description: "d"}}); err == nil {
			t.Fatal("expected a failure after the retry also fails")
		}
	})

	t.Run("400 is never retried", func(t *testing.T) {
		var calls int32
		server, client := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&calls, 1)
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "bad request"}})
		})
		_ = server
		if _, err := client.Score(context.Background(), "ctx", []Item{{Path: "a.md", Title: "A", Description: "d"}}); err == nil {
			t.Fatal("expected a failure")
		}
		if got := atomic.LoadInt32(&calls); got != 1 {
			t.Errorf("a 400 was retried %d times, want exactly 1 attempt", got)
		}
	})
}

func TestScoreFailsOnMalformedOrMissingAnswer(t *testing.T) {
	t.Run("malformed body", func(t *testing.T) {
		server, client := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, "not json")
		})
		_ = server
		if _, err := client.Score(context.Background(), "ctx", []Item{{Path: "a.md", Title: "A", Description: "d"}}); err == nil {
			t.Fatal("expected a failure for an unparseable response")
		}
	})

	t.Run("missing answer", func(t *testing.T) {
		server, client := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"answers": map[string]any{}})
		})
		_ = server
		if _, err := client.Score(context.Background(), "ctx", []Item{{Path: "a.md", Title: "A", Description: "d"}}); err == nil {
			t.Fatal("expected a failure when a question's answer is missing")
		}
	})
}

func TestScoreRespectsTheParallelCap(t *testing.T) {
	var inFlight, maxSeen int32
	var mu sync.Mutex
	server, client := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&inFlight, 1)
		mu.Lock()
		if n > maxSeen {
			maxSeen = n
		}
		mu.Unlock()
		time.Sleep(50 * time.Millisecond)
		questions := decodeRequest(t, r)
		names := make([]string, 0, len(questions))
		for name := range questions {
			names = append(names, name)
		}
		writeAnswers(w, names, func(string) float64 { return 0.1 })
		atomic.AddInt32(&inFlight, -1)
	})
	_ = server

	client.Parallel = 2
	client.Budget = 100 // forces one question per chunk, many chunks

	items := make([]Item, 0, 8)
	for i := 0; i < 8; i++ {
		items = append(items, Item{Path: fmt.Sprintf("r%d.md", i), Title: "T", Description: "d"})
	}
	if _, err := client.Score(context.Background(), "ctx", items); err != nil {
		t.Fatalf("Score: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if maxSeen > 2 {
		t.Errorf("max concurrent requests = %d, want at most 2", maxSeen)
	}
}

func TestResolveEndpointGuardsHttpToNonLoopback(t *testing.T) {
	if _, err := resolveEndpoint("http://example.com/jev"); err == nil {
		t.Error("expected http:// to a non-loopback host to be refused")
	}
	if _, err := resolveEndpoint("http://127.0.0.1:8080/jev"); err != nil {
		t.Errorf("http:// to loopback should be allowed: %v", err)
	}
	if _, err := resolveEndpoint("http://localhost:8080/jev"); err != nil {
		t.Errorf("http:// to localhost should be allowed: %v", err)
	}
	if _, err := resolveEndpoint("https://example.com/jev"); err != nil {
		t.Errorf("https:// should always be allowed: %v", err)
	}
}

func TestFromEnvRequiresTheKey(t *testing.T) {
	env := map[string]string{}
	getenv := func(name string) string { return env[name] }
	if _, err := FromEnv(getenv); err == nil {
		t.Error("expected an error with no OPENROUTER_API_KEY")
	}
	env["OPENROUTER_API_KEY"] = "k"
	client, err := FromEnv(getenv)
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if client.Endpoint != DefaultEndpoint {
		t.Errorf("Endpoint = %q, want the default", client.Endpoint)
	}
}

func TestFromEnvHonoursTheEndpointSeam(t *testing.T) {
	env := map[string]string{
		"OPENROUTER_API_KEY":      "k",
		"OPENRECORD_JEV_ENDPOINT": "http://127.0.0.1:9/jev",
	}
	getenv := func(name string) string { return env[name] }
	client, err := FromEnv(getenv)
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if client.Endpoint != "http://127.0.0.1:9/jev" {
		t.Errorf("Endpoint = %q, want the override", client.Endpoint)
	}
}

func TestProbeWithoutKeySendsNoRequest(t *testing.T) {
	var called bool
	server, client := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	_ = server
	client.Key = ""
	status := client.Probe(context.Background())
	if status.APIKey {
		t.Error("api_key = true with no key set")
	}
	if status.Reachable {
		t.Error("reachable = true with no key set")
	}
	if called {
		t.Error("a request was sent with no key")
	}
}

func TestProbeHealthy(t *testing.T) {
	server, client := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
		questions := decodeRequest(t, r)
		names := make([]string, 0, len(questions))
		for name := range questions {
			names = append(names, name)
		}
		writeAnswers(w, names, func(string) float64 { return 0.1 })
	})
	_ = server
	status := client.Probe(context.Background())
	if !status.APIKey || !status.Reachable {
		t.Errorf("status = %+v, want api_key and reachable both true", status)
	}
	if status.Model != Model {
		t.Errorf("Model = %q, want %q", status.Model, Model)
	}
}

func TestProbeUnreachable(t *testing.T) {
	server, client := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	_ = server
	status := client.Probe(context.Background())
	if !status.APIKey {
		t.Error("api_key = false despite a key being set")
	}
	if status.Reachable {
		t.Error("reachable = true despite every call failing")
	}
	if status.Trouble == "" {
		t.Error("Trouble is empty for an unreachable endpoint")
	}
}
