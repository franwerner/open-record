package e2e

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The command prints the embedded document verbatim — never rebuilt from
// parsed entries — so its stdout has to match the source file byte for byte.
// Read from "../docs/verification.md" rather than compared against anything
// parsed: go:embed in assets.go picks up exactly this path, at this module
// root, so this is the one file a drift could actually happen against.
func TestVerificationPrintsTheDocumentVerbatim(t *testing.T) {
	repo := project(t)
	stdout := mustRun(t, repo, "verification")

	want, err := os.ReadFile("../docs/verification.md")
	if err != nil {
		t.Fatalf("read docs/verification.md: %v", err)
	}
	if stdout != string(want) {
		t.Errorf("`openrecord verification` did not print docs/verification.md byte for byte\n"+
			"got %d bytes, want %d bytes", len(stdout), len(want))
	}
}

// concernHeadingPattern and concernTopicPattern (the `## concern` heading and
// the `- **topic**` bullet) are already declared in concerns_test.go, in this
// same package, and are reused below rather than redeclared.
var (
	verificationGateHeading  = regexp.MustCompile(`(?m)^##\s+Gate\s+([123])\b`)
	verificationEntryHeading = regexp.MustCompile(`(?m)^###\s+([a-z][a-z0-9/-]*)\s*$`)
)

// verificationMarker is one heading found in the printed catalogue, kept with
// its byte position so gate headings and entry headings — found by two
// separate regexps — can be merged back into document order.
type verificationMarker struct {
	pos  int
	gate string
	key  string
}

// keysByGate walks the printed catalogue once and buckets every entry key
// under the gate heading that precedes it, the same way the production
// parser in internal/verification does — but re-derived here from the
// command's own stdout, never imported, so a change to that package's
// internals cannot silently go unnoticed by this suite.
func keysByGate(t *testing.T, document string) map[string][]string {
	t.Helper()
	var markers []verificationMarker
	for _, m := range verificationGateHeading.FindAllStringSubmatchIndex(document, -1) {
		markers = append(markers, verificationMarker{pos: m[0], gate: document[m[2]:m[3]]})
	}
	for _, m := range verificationEntryHeading.FindAllStringSubmatchIndex(document, -1) {
		markers = append(markers, verificationMarker{pos: m[0], key: document[m[2]:m[3]]})
	}
	sort.Slice(markers, func(i, j int) bool { return markers[i].pos < markers[j].pos })

	byGate := map[string][]string{}
	current := ""
	for _, m := range markers {
		if m.gate != "" {
			current = m.gate
			continue
		}
		if current == "" {
			continue
		}
		byGate[current] = append(byGate[current], m.key)
	}
	return byGate
}

// Gate 3's keys must correspond, one for one, to the sixty topics of
// docs/concerns.md: none missing, none extra. This is read against the
// `concerns` command's own output rather than the file on disk, which is
// exactly what concerns_test.go already asserts elsewhere — the topic list
// here is derived the same way, from the live command, not re-parsed from a
// source file this suite does not own.
func TestGate3KeysMatchExactlyTheConcernsTopics(t *testing.T) {
	repo := project(t)

	concernsOut := mustRun(t, repo, "concerns")
	wantTopics := map[string]bool{}
	current := ""
	for _, line := range strings.Split(concernsOut, "\n") {
		if m := concernHeadingPattern.FindStringSubmatch(line); m != nil {
			current = m[1]
			continue
		}
		if current == "" {
			continue
		}
		if m := concernTopicPattern.FindStringSubmatch(line); m != nil {
			wantTopics[current+"/"+m[1]] = true
		}
	}
	if len(wantTopics) != 60 {
		t.Fatalf("derived %d topics from `openrecord concerns`, want 60", len(wantTopics))
	}

	verificationOut := mustRun(t, repo, "verification")
	byGate := keysByGate(t, verificationOut)
	gate3 := byGate["3"]
	if len(gate3) != 60 {
		t.Fatalf("gate 3 has %d entries, want 60: %v", len(gate3), gate3)
	}

	gotKeys := map[string]bool{}
	for _, key := range gate3 {
		gotKeys[key] = true
	}
	for topic := range wantTopics {
		if !gotKeys[topic] {
			t.Errorf("concerns.md topic %q has no gate 3 entry", topic)
		}
	}
	for key := range gotKeys {
		if !wantTopics[key] {
			t.Errorf("gate 3 entry %q does not correspond to any concerns.md topic", key)
		}
	}
}

var (
	verificationLandingPattern = regexp.MustCompile(`-\s+\*\*Where it lands\*\*\s+(\S+)`)
	verificationClassPattern   = regexp.MustCompile(`-\s+\*\*Class\*\*\s+(\S+)`)
)

// Every entry's "Where it lands" and "Class" fields are read straight out of
// the printed document, by key, and checked against the five/three closed
// vocabularies this command's own documentation promises — proof that the
// catalogue is well-formed from the consumer's side, not just the parser's.
func TestEveryEntryHasAValidLandingAndClass(t *testing.T) {
	repo := project(t)
	document := mustRun(t, repo, "verification")

	landings := map[string]bool{"on-change": true, "pre-merge": true, "pre-release": true, "production": true, "manual": true}
	classes := []string{"full-check", "partial-check+audit", "manual-only"}

	keys := verificationEntryHeading.FindAllStringSubmatch(document, -1)
	if len(keys) != 11+16+60 {
		t.Fatalf("found %d entry headings, want %d", len(keys), 11+16+60)
	}

	for _, m := range keys {
		key := m[1]
		block := verificationOutputFor(t, repo, key)

		landing := verificationLandingPattern.FindStringSubmatch(block)
		if landing == nil {
			t.Errorf("%q has no \"Where it lands\" field", key)
			continue
		}
		if !landings[landing[1]] {
			t.Errorf("%q: %q is not a valid landing value", key, landing[1])
		}

		class := verificationClassPattern.FindStringSubmatch(block)
		if class == nil {
			t.Errorf("%q has no \"Class\" field", key)
			continue
		}
		found := false
		for _, want := range classes {
			if strings.HasPrefix(class[1], strings.SplitN(want, "+", 2)[0]) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%q: %q is not a valid class value", key, class[1])
		}
	}
}

func verificationOutputFor(t *testing.T, repo, key string) string {
	t.Helper()
	return mustRun(t, repo, "verification", key)
}

// A key that exists prints only that entry, verbatim — never the whole
// document, and never bleeding into the next heading.
func TestVerificationWithAKeyPrintsOnlyThatEntry(t *testing.T) {
	repo := project(t)
	stdout := mustRun(t, repo, "verification", "security/rate-limiting")

	if !strings.HasPrefix(stdout, "### security/rate-limiting") {
		t.Fatalf("did not start with its own heading:\n%s", stdout)
	}
	if strings.Count(stdout, "\n### ") != 0 {
		t.Errorf("printed more than one entry:\n%s", stdout)
	}
	if strings.Contains(stdout, "## Gate") {
		t.Errorf("bled into a gate heading:\n%s", stdout)
	}
	if !strings.Contains(stdout, "the limit") {
		t.Errorf("entry does not look like rate limiting:\n%s", stdout)
	}
}

// An unknown key is a usage error (not a crash, not exit 0), and the message
// names at least one real key so a caller can self-correct.
func TestVerificationWithAnUnknownKeyListsValidKeys(t *testing.T) {
	repo := project(t)
	code, stdout, stderr := run(t, repo, "verification", "not-a-real-key")
	if code == exitOK {
		t.Fatalf("an unknown key was accepted; stdout: %s", stdout)
	}
	if !strings.Contains(stderr, "valid keys") {
		t.Errorf("error does not mention valid keys: %s", stderr)
	}
	// It has to be gate 2's own list that comes back here: "not-a-real-key" has
	// no "/", and it is not the one gate-1 exception ("rule"), so by the
	// catalogue's own GateFor rule it reads as a quality-attribute typo.
	if !strings.Contains(stderr, "latency") {
		t.Errorf("error does not list a gate 2 key for a bare unknown key: %s", stderr)
	}
}

// The catalogue closes with the one section every gate shares, stating what
// no mechanism above it ever proves.
func TestWhatNoMechanismProvesSectionExists(t *testing.T) {
	repo := project(t)
	stdout := mustRun(t, repo, "verification")
	if !strings.Contains(stdout, "## What no mechanism proves") {
		t.Error("the catalogue is missing its closing section")
	}
}
