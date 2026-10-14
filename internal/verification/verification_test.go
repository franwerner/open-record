package verification

import (
	"strings"
	"testing"
)

// The catalogue's three gates ship a fixed number of entries: eleven aspects
// of a functional spec, sixteen quality attributes, and one entry per
// concerns.md topic (sixty, asserted against the real document in
// e2e/verification_test.go rather than hardcoded here, since that is the
// invariant actually worth protecting).
func TestGateCounts(t *testing.T) {
	cases := []struct {
		gate Gate
		want int
	}{
		{GateFunctional, 11},
		{GateQuality, 16},
		{GateDecisions, 60},
	}
	for _, c := range cases {
		if got := len(KeysForGate(c.gate)); got != c.want {
			t.Errorf("gate %q has %d entries, want %d", c.gate, got, c.want)
		}
	}
	if got, want := len(Keys()), 11+16+60; got != want {
		t.Errorf("Keys() returned %d entries total, want %d", got, want)
	}
}

func TestLoadErrIsNilOnAWorkingBuild(t *testing.T) {
	if err := LoadErr(); err != nil {
		t.Fatalf("the embedded verification catalogue failed to load: %v", err)
	}
}

func TestLookupFoldsCase(t *testing.T) {
	entry, ok := Lookup("Security/Rate-Limiting")
	if !ok {
		t.Fatal("Lookup did not fold case")
	}
	if entry.Key != "security/rate-limiting" {
		t.Errorf("Key = %q, want the lowercase form", entry.Key)
	}
}

func TestLookupReturnsExactlyOneEntryVerbatim(t *testing.T) {
	entry, ok := Lookup("security/rate-limiting")
	if !ok {
		t.Fatal("security/rate-limiting not found")
	}
	if !strings.HasPrefix(entry.Raw, "### security/rate-limiting") {
		t.Errorf("Raw does not start with its own heading: %q", entry.Raw)
	}
	// A sliced entry never carries the next one's heading — otherwise two
	// `verification <key>` calls in a row would print overlapping content.
	if strings.Count(entry.Raw, "\n### ") != 0 {
		t.Errorf("Raw bleeds into the next entry: %q", entry.Raw)
	}
	if strings.Contains(entry.Raw, "## Gate") {
		t.Errorf("Raw bleeds into a gate heading: %q", entry.Raw)
	}
}

func TestUnknownKeyIsNotAnError(t *testing.T) {
	if _, ok := Lookup("no-such-key"); ok {
		t.Error("an invented key was found in the catalogue")
	}
}

// GateFor is read only when Lookup already missed, to decide which gate's
// keys an unknown-key error should list.
func TestGateFor(t *testing.T) {
	cases := []struct {
		key       string
		wantGate  Gate
		wantMatch bool
	}{
		{"flow/typo", GateFunctional, true},
		{"process/typo", GateFunctional, true},
		{"lifecycle/typo", GateFunctional, true},
		{"all/typo", GateFunctional, true},
		{"rule", GateFunctional, true}, // gate 1's one bare key
		{"security/typo", GateDecisions, true},
		{"structure/typo", GateDecisions, true},
		{"delivery/typo", GateDecisions, true},
		{"latancy", GateQuality, true}, // bare word, not "rule" -> gate 2 guess
		{"throughpuut", GateQuality, true},
		{"nonsense/typo", "", false}, // a "/" whose prefix matches neither vocabulary
	}
	for _, c := range cases {
		gate, matched := GateFor(c.key)
		if matched != c.wantMatch {
			t.Errorf("GateFor(%q) matched = %v, want %v", c.key, matched, c.wantMatch)
			continue
		}
		if matched && gate != c.wantGate {
			t.Errorf("GateFor(%q) = %q, want %q", c.key, gate, c.wantGate)
		}
	}
}

func TestEveryEntryHasAValidLandingAndClass(t *testing.T) {
	landings := map[string]bool{"on-change": true, "pre-merge": true, "pre-release": true, "production": true, "manual": true}
	classes := []string{"full-check", "partial-check+audit", "manual-only"}

	for _, key := range Keys() {
		entry, _ := Lookup(key)

		landingLine := fieldLine(t, entry.Raw, "Where it lands")
		landing := strings.TrimSpace(landingLine)
		if !landings[landing] {
			t.Errorf("%q: %q is not a valid landing value", key, landing)
		}

		classLine := fieldLine(t, entry.Raw, "Class")
		found := false
		for _, class := range classes {
			if strings.HasPrefix(classLine, class) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%q: %q does not start with a valid class", key, classLine)
		}
	}
}

// fieldLine pulls the value after "- **<field>** " on its own line, failing
// the test outright if the entry has no such field — every entry is expected
// to carry "Where it lands" and "Class".
func fieldLine(t *testing.T, raw, field string) string {
	t.Helper()
	marker := "**" + field + "**"
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- "+marker) {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "- "+marker))
		}
	}
	t.Fatalf("no %q field found in: %s", field, raw)
	return ""
}

func TestGate1KeysHaveNoQuestionInGate3(t *testing.T) {
	// Gate 3 deliberately has no Question field: the question lives in
	// concerns.md and is not duplicated here.
	for _, key := range KeysForGate(GateDecisions) {
		entry, _ := Lookup(key)
		if strings.Contains(entry.Raw, "**Question**") {
			t.Errorf("%q (gate 3) carries a Question field, which should live in concerns.md instead", key)
		}
	}
	for _, key := range KeysForGate(GateFunctional) {
		entry, _ := Lookup(key)
		if !strings.Contains(entry.Raw, "**Question**") {
			t.Errorf("%q (gate 1) is missing its Question field", key)
		}
	}
	for _, key := range KeysForGate(GateQuality) {
		entry, _ := Lookup(key)
		if !strings.Contains(entry.Raw, "**Question**") {
			t.Errorf("%q (gate 2) is missing its Question field", key)
		}
	}
}
