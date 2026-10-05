// Package jev is a thin HTTP client to the Jev decisions API: the model that
// scores whether a context touches what each record decides.
//
// Shaped like internal/qmd — its own probe, its own time bound — but jev is
// never optional the way qmd once was: search requires it, and there is no
// degraded mode. Every way a request can fail to produce a trustworthy score
// is reported as an error; nothing here silently serves a partial result.
package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Model is the one Jev release openrecord is pinned to.
const Model = "typesafe/jev-1.13-20260917"

// DefaultEndpoint is where Jev lives, unless OPENRECORD_JEV_ENDPOINT
// overrides it for a trusted local double.
const DefaultEndpoint = "https://openrouter.ai/api/alpha/decisions"

// envEndpoint is the seam a cli test or e2e run uses to redirect requests at
// a local double. Only the environment reaches the built binary, so this one
// variable covers every layer that needs to intercept a Jev call.
const envEndpoint = "OPENRECORD_JEV_ENDPOINT"

const (
	defaultParallel = 4
	defaultBudget   = 32000
	requestTimeout  = 90 * time.Second
	// perQuestionOverhead is what one question costs beyond its own name and
	// instructions text: the envelope syntax around it in the request body.
	perQuestionOverhead = 34
)

// Item is one record to be scored: its path, title and description. Bodies
// are never sent — Jev scores what a record is about, not what it says.
type Item struct {
	Path        string
	Title       string
	Description string
}

// Client talks to one Jev endpoint with one key.
type Client struct {
	Endpoint string
	Key      string
	HTTP     *http.Client
	// Parallel caps how many chunk requests run at once. Zero uses the
	// default of 4.
	Parallel int
	// Budget is the token budget one request is planned against. Zero uses
	// the default of 32000.
	Budget int
}

// FromEnv builds a Client from the environment: the key, required, and the
// endpoint override seam, which is refused when it would send the key in
// cleartext over anything but loopback.
func FromEnv(getenv func(string) string) (Client, error) {
	key := strings.TrimSpace(getenv("OPENROUTER_API_KEY"))
	if key == "" {
		return Client{}, errors.New("OPENROUTER_API_KEY is unset or empty")
	}
	endpoint, err := EndpointFromEnv(getenv)
	if err != nil {
		return Client{}, err
	}
	return Client{Endpoint: endpoint, Key: key}, nil
}

// EndpointFromEnv resolves the endpoint a Client should use: the default,
// unless OPENRECORD_JEV_ENDPOINT overrides it. It does not require a key —
// `jev status` needs to know (and report) the endpoint it would talk to even
// with no key set.
func EndpointFromEnv(getenv func(string) string) (string, error) {
	endpoint := DefaultEndpoint
	if override := strings.TrimSpace(getenv(envEndpoint)); override != "" {
		resolved, err := resolveEndpoint(override)
		if err != nil {
			return "", err
		}
		endpoint = resolved
	}
	return endpoint, nil
}

// resolveEndpoint allows http:// only to a loopback host. An https:// URL is
// always accepted without inspecting its host.
func resolveEndpoint(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("%s: %v", envEndpoint, err)
	}
	if parsed.Scheme == "http" && !isLoopback(parsed.Hostname()) {
		return "", fmt.Errorf("%s must be https, or http to a loopback host; got %q", envEndpoint, raw)
	}
	return raw, nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (c Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: requestTimeout}
}

func (c Client) parallel() int {
	if c.Parallel > 0 {
		return c.Parallel
	}
	return defaultParallel
}

func (c Client) budget() int {
	if c.Budget > 0 {
		return c.Budget
	}
	return defaultBudget
}

// Error is a Jev request that failed with a response this client could read
// enough of to report. Code is the machine-readable cause Jev's own error
// body names, when it names one; Message is always set, falling back to the
// raw body when there is no parseable code or message.
type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("jev: HTTP %d: %s", e.Status, e.Message)
	}
	return fmt.Sprintf("jev: HTTP %d", e.Status)
}

// questionNamePattern is what a path is reduced to for a question name:
// Jev's question keys must be identifier-like, so anything outside
// [A-Za-z0-9] collapses to one underscore.
var questionNamePattern = regexp.MustCompile(`[^A-Za-z0-9]+`)

// question is one item turned into Jev's wire shape.
type question struct {
	Name         string
	Instructions string
}

// buildQuestions names and writes the instruction for every item, resolving
// name collisions deterministically: the first item with a given base name
// keeps it, and every later collision gets _2, _3 and so on, in item order.
func buildQuestions(items []Item) []question {
	seen := make(map[string]int, len(items))
	questions := make([]question, len(items))
	for index, item := range items {
		base := strings.Trim(questionNamePattern.ReplaceAllString(item.Path, "_"), "_")
		if base == "" {
			base = "record"
		}
		seen[base]++
		name := base
		if count := seen[base]; count > 1 {
			name = fmt.Sprintf("%s_%d", base, count)
		}
		questions[index] = question{Name: name, Instructions: instructionFor(item)}
	}
	return questions
}

// instructionFor is the one wording Jev is asked, pinned to the benchmarked
// phrasing with "task" changed to "context". An empty title falls back to
// the path, and a trailing "." on the description is trimmed so the
// generated sentence never carries a double stop.
func instructionFor(item Item) string {
	title := strings.TrimSpace(item.Title)
	if title == "" {
		title = item.Path
	}
	description := strings.TrimRight(strings.TrimSpace(item.Description), ".")
	return fmt.Sprintf(
		`Record %q: %s. Does the context touch what this record decides, even if the context goes against it?`,
		title, description)
}

// estimateTokens is the one estimate this package trusts: runes divided by
// 3.5, rounded up. It deliberately overcounts rather than undercounts, so a
// refusal from Jev signals a genuinely changed limit rather than this
// package's own math being wrong in the unsafe direction.
func estimateTokens(text string) int {
	runes := len([]rune(text))
	return int(math.Ceil(float64(runes) / 3.5))
}

func questionTokens(q question) int {
	return estimateTokens(q.Name) + estimateTokens(q.Instructions) + perQuestionOverhead
}

// chunkPlan splits questions into the smallest number of contiguous,
// near-equal chunks that each fit the token budget. It fails, naming
// "context too long", when even one question alone would not fit — no split
// helps that.
func chunkPlan(questions []question, stateTokens, budget int) ([][]question, error) {
	if len(questions) == 0 {
		return nil, nil
	}
	unit := budget - stateTokens
	if unit <= 0 {
		return nil, fmt.Errorf("context too long: the budget leaves no room for any question")
	}

	total := 0
	largest := 0
	for _, q := range questions {
		tokens := questionTokens(q)
		total += tokens
		if tokens > largest {
			largest = tokens
		}
	}
	if largest > unit {
		return nil, fmt.Errorf("context too long: the largest question needs %d tokens and the budget allows %d", largest, unit)
	}

	n := int(math.Ceil(float64(total) / float64(unit)))
	if n < 1 {
		n = 1
	}
	for {
		chunks := splitInto(questions, n)
		if fitsEvery(chunks, unit) {
			return chunks, nil
		}
		n++
	}
}

// splitInto divides questions into n contiguous groups whose sizes differ by
// at most 1 — the first len%n groups get one extra item.
func splitInto(questions []question, n int) [][]question {
	if n <= 0 {
		n = 1
	}
	total := len(questions)
	base := total / n
	extra := total % n
	chunks := make([][]question, 0, n)
	index := 0
	for i := 0; i < n; i++ {
		size := base
		if i < extra {
			size++
		}
		if size == 0 {
			continue
		}
		chunks = append(chunks, questions[index:index+size])
		index += size
	}
	return chunks
}

func fitsEvery(chunks [][]question, unit int) bool {
	for _, chunk := range chunks {
		sum := 0
		for _, q := range chunk {
			sum += questionTokens(q)
		}
		if sum > unit {
			return false
		}
	}
	return true
}

// Score sends every item to Jev, chunked to fit the budget, and merges the
// per-chunk answers into one map keyed by path. Up to Client.parallel()
// chunks run at once; the first chunk to fail cancels the rest, since a
// partial score is never a usable one.
func (c Client) Score(ctx context.Context, contextText string, items []Item) (map[string]float64, error) {
	if len(items) == 0 {
		return map[string]float64{}, nil
	}
	questions := buildQuestions(items)
	nameToPath := make(map[string]string, len(items))
	for index, q := range questions {
		nameToPath[q.Name] = items[index].Path
	}

	state := map[string]string{"context": contextText}
	stateTokens := estimateTokens(contextText)

	chunks, err := chunkPlan(questions, stateTokens, c.budget())
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type result struct {
		scores map[string]float64
		err    error
	}
	results := make([]result, len(chunks))
	sem := make(chan struct{}, c.parallel())
	var wg sync.WaitGroup
	var failOnce sync.Once
	var firstErr error

	for index, chunk := range chunks {
		wg.Add(1)
		sem <- struct{}{}
		go func(index int, chunk []question) {
			defer wg.Done()
			defer func() { <-sem }()
			scores, chunkErr := c.scoreChunk(ctx, state, chunk, nameToPath, false)
			results[index] = result{scores: scores, err: chunkErr}
			if chunkErr != nil {
				failOnce.Do(func() {
					firstErr = chunkErr
					cancel()
				})
			}
		}(index, chunk)
	}
	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}
	merged := make(map[string]float64, len(items))
	for _, r := range results {
		for path, score := range r.scores {
			merged[path] = score
		}
	}
	return merged, nil
}

// scoreChunk sends one request for one chunk, applying the split-once and
// one-retry policy, and decodes the answers for exactly the questions it
// asked. alreadySplit is true once a half from an earlier max_tokens_exceeded
// split is being retried — a second refusal at that point is a hard failure,
// since the policy only ever splits once.
func (c Client) scoreChunk(ctx context.Context, state map[string]string, chunk []question, nameToPath map[string]string, alreadySplit bool) (map[string]float64, error) {
	answers, err := c.send(ctx, state, chunk)
	if err != nil {
		if !alreadySplit && len(chunk) > 1 && isMaxTokensExceeded(err) {
			mid := len(chunk) / 2
			first, firstErr := c.scoreChunk(ctx, state, chunk[:mid], nameToPath, true)
			if firstErr != nil {
				return nil, firstErr
			}
			second, secondErr := c.scoreChunk(ctx, state, chunk[mid:], nameToPath, true)
			if secondErr != nil {
				return nil, secondErr
			}
			merged := make(map[string]float64, len(first)+len(second))
			for k, v := range first {
				merged[k] = v
			}
			for k, v := range second {
				merged[k] = v
			}
			return merged, nil
		}
		return nil, err
	}

	scores := make(map[string]float64, len(chunk))
	for _, q := range chunk {
		path := nameToPath[q.Name]
		score, ok := answers[q.Name]
		if !ok {
			return nil, fmt.Errorf("jev: no answer for %s (%s)", q.Name, path)
		}
		scores[path] = score
	}
	return scores, nil
}

func isMaxTokensExceeded(err error) bool {
	var jerr *Error
	if !errors.As(err, &jerr) {
		return false
	}
	return jerr.Status == http.StatusBadRequest && strings.Contains(jerr.Message, "max_tokens_exceeded")
}

// send issues one HTTP request for one chunk of questions, retrying once
// after 1s on a 429, a 5xx, or a transport error — the three cases a second
// attempt can plausibly fix. A 400 is never retried here; the split-once
// policy in scoreChunk is the only thing that can rescue it.
func (c Client) send(ctx context.Context, state map[string]string, chunk []question) (map[string]float64, error) {
	questions := make(map[string]any, len(chunk))
	for _, q := range chunk {
		questions[q.Name] = map[string]string{"type": "noul", "instructions": q.Instructions}
	}
	body, err := json.Marshal(map[string]any{
		"model":     Model,
		"state":     state,
		"questions": questions,
	})
	if err != nil {
		return nil, fmt.Errorf("jev: encoding request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= 1; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(time.Second):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		answers, doErr := c.do(ctx, body)
		if doErr == nil {
			return answers, nil
		}
		lastErr = doErr
		if !retryable(doErr) {
			return nil, doErr
		}
	}
	return nil, lastErr
}

// retryable reports whether a failure is one the one-retry policy covers: a
// 429, a 5xx, or anything that is not a parsed *Error at all — a dial error,
// a timeout, a connection reset before a response ever arrived.
func retryable(err error) bool {
	var jerr *Error
	if errors.As(err, &jerr) {
		return jerr.Status == http.StatusTooManyRequests || jerr.Status >= 500
	}
	return true
}

func (c Client) do(ctx context.Context, body []byte) (map[string]float64, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+c.Key)
	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient().Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, parseError(response.StatusCode, raw)
	}

	var decoded struct {
		Answers map[string]struct {
			Noul *float64 `json:"noul"`
		} `json:"answers"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("jev: malformed response: %v", err)
	}
	scores := make(map[string]float64, len(decoded.Answers))
	for name, answer := range decoded.Answers {
		if answer.Noul == nil {
			return nil, fmt.Errorf("jev: missing answer for %s", name)
		}
		scores[name] = *answer.Noul
	}
	return scores, nil
}

// parseError builds an Error from a non-2xx response. Jev's error body is
// read as `{"error":{"code":"...","message":"..."}}` when it is that shape;
// anything else falls back to the full raw body, trimmed — never just its
// first line, since the one thing a caller needs to find inside it, like
// "max_tokens_exceeded", is not guaranteed to be on the first line.
func parseError(status int, raw []byte) *Error {
	var decoded struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &decoded) == nil && decoded.Error.Message != "" {
		message := decoded.Error.Message
		if decoded.Error.Code != "" && !strings.Contains(message, decoded.Error.Code) {
			message = decoded.Error.Code + ": " + message
		}
		return &Error{Status: status, Code: decoded.Error.Code, Message: message}
	}
	return &Error{Status: status, Message: strings.TrimSpace(string(raw))}
}

// Status is what Probe found.
type Status struct {
	APIKey    bool   `json:"api_key"`
	Reachable bool   `json:"reachable"`
	Model     string `json:"model"`
	Endpoint  string `json:"endpoint"`
	Trouble   string `json:"trouble,omitempty"`
}

// Probe asks whether the endpoint answers one minimal request. Without a key
// it reports api_key: false without sending anything — there is no request a
// missing key could plausibly send.
func (c Client) Probe(ctx context.Context) Status {
	status := Status{Model: Model, Endpoint: c.Endpoint}
	if strings.TrimSpace(c.Key) == "" {
		return status
	}
	status.APIKey = true

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	_, err := c.Score(ctx, "probe", []Item{{Path: "probe", Title: "probe", Description: "a reachability probe"}})
	if err != nil {
		status.Trouble = err.Error()
		return status
	}
	status.Reachable = true
	return status
}
