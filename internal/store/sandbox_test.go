package store

import "testing"

func wellFormedSandbox(id string) Sandbox {
	return Sandbox{
		ID:          id,
		Description: "d",
		Setup:       []string{"docker compose up -d"},
		Ready:       Ready{Http: "http://localhost:8080/health", Timeout: 60},
		Logs:        Logs{Source: "docker compose logs api", Format: "json"},
		Stop:        "docker compose down -v",
	}
}

func TestLoadSandboxesReportsAnUndeclaredStoreAsBenign(t *testing.T) {
	repo := t.TempDir()
	_, err := LoadSandboxes(repo)
	if err == nil {
		t.Fatal("LoadSandboxes on an empty repo returned no error")
	}
	if !IsSandboxesUndeclared(err) {
		t.Errorf("IsSandboxesUndeclared(%v) = false, want true", err)
	}
}

func TestSandboxesSaveAndLoadRoundTrip(t *testing.T) {
	repo := t.TempDir()
	set := Sandboxes{Sandboxes: []Sandbox{wellFormedSandbox("api")}}
	if err := set.Save(repo); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadSandboxes(repo)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Hash == "" {
		t.Error("Save did not stamp a hash")
	}
	if findings := VerifySandboxes(repo, reloaded); len(findings) != 0 {
		t.Errorf("VerifySandboxes rejected what Save wrote: %+v", findings)
	}
	if !reloaded.Has("api") || !reloaded.Has("API") {
		t.Error("Has is not case-insensitive")
	}
	item, ok := reloaded.Get("api")
	if !ok || item.Stop != "docker compose down -v" {
		t.Errorf("Get returned %+v, ok=%v", item, ok)
	}
}

func TestSandboxesIDsAreSorted(t *testing.T) {
	set := Sandboxes{Sandboxes: []Sandbox{wellFormedSandbox("web"), wellFormedSandbox("api")}}
	ids := set.IDs()
	if len(ids) != 2 || ids[0] != "api" || ids[1] != "web" {
		t.Errorf("IDs = %v, want sorted [api web]", ids)
	}
}

func TestSandboxesValidateAcceptsAWellFormedDeclaration(t *testing.T) {
	set := Sandboxes{Sandboxes: []Sandbox{wellFormedSandbox("api")}}
	if findings := set.Validate(); len(findings) != 0 {
		t.Errorf("a well-formed declaration was rejected: %+v", findings)
	}
}

func TestSandboxesValidateReportsADuplicateID(t *testing.T) {
	set := Sandboxes{Sandboxes: []Sandbox{wellFormedSandbox("api"), wellFormedSandbox("API")}}
	findings := set.Validate()
	if len(findings) != 1 || findings[0].Code != "sandbox-duplicate" {
		t.Fatalf("Validate(duplicate id) = %+v, want exactly one sandbox-duplicate", findings)
	}
}

func TestSandboxesValidateReportsAMissingDescription(t *testing.T) {
	item := wellFormedSandbox("api")
	item.Description = ""
	set := Sandboxes{Sandboxes: []Sandbox{item}}
	findings := set.Validate()
	if len(findings) != 1 || findings[0].Code != "sandbox-without-description" {
		t.Fatalf("Validate(no description) = %+v, want exactly one sandbox-without-description", findings)
	}
}

func TestSandboxesValidateReportsNeitherSetupNorStart(t *testing.T) {
	item := wellFormedSandbox("api")
	item.Setup = nil
	set := Sandboxes{Sandboxes: []Sandbox{item}}
	findings := set.Validate()
	if len(findings) != 1 || findings[0].Code != "sandbox-without-setup-or-start" {
		t.Fatalf("Validate(no setup or start) = %+v, want exactly one sandbox-without-setup-or-start", findings)
	}
}

func TestSandboxesValidateAcceptsStartAloneWithNoSetup(t *testing.T) {
	item := wellFormedSandbox("api")
	item.Setup = nil
	item.Start = "docker compose up"
	set := Sandboxes{Sandboxes: []Sandbox{item}}
	if findings := set.Validate(); len(findings) != 0 {
		t.Errorf("start alone was rejected: %+v", findings)
	}
}

func TestSandboxesValidateReportsNoReadyAtAll(t *testing.T) {
	item := wellFormedSandbox("api")
	item.Ready = Ready{}
	set := Sandboxes{Sandboxes: []Sandbox{item}}
	findings := set.Validate()
	if len(findings) != 1 || findings[0].Code != "sandbox-ready-missing" {
		t.Fatalf("Validate(no ready) = %+v, want exactly one sandbox-ready-missing", findings)
	}
}

func TestSandboxesValidateReportsBothHttpAndLog(t *testing.T) {
	item := wellFormedSandbox("api")
	item.Ready.Log = "listening"
	set := Sandboxes{Sandboxes: []Sandbox{item}}
	findings := set.Validate()
	if len(findings) != 1 || findings[0].Code != "sandbox-ready-ambiguous" {
		t.Fatalf("Validate(http and log) = %+v, want exactly one sandbox-ready-ambiguous", findings)
	}
}

// ready.timeout and logs.format are optional (DefaultReadyTimeout,
// DefaultLogsFormat apply when absent) — absence is accepted, a present
// value is still checked for shape. ready.timeout is a whole number of
// seconds, never a duration string; it cannot be hand-constructed as
// "not an integer" the way a string-typed field could — the one way the
// in-memory value itself can still be wrong is negative, since `sandbox
// add`/`sandbox edit` (internal/cli) are what reject zero, a negative
// number and non-integer text at the flag-parsing boundary, where the
// raw text is still available to tell "0" apart from "not given".

func TestSandboxesValidateAcceptsNoTimeoutOrFormatAtAll(t *testing.T) {
	item := wellFormedSandbox("api")
	item.Ready.Timeout = 0
	item.Logs.Format = ""
	set := Sandboxes{Sandboxes: []Sandbox{item}}
	if findings := set.Validate(); len(findings) != 0 {
		t.Errorf("an absent ready.timeout/logs.format was rejected: %+v", findings)
	}
}

func TestSandboxesValidateReportsANegativeTimeout(t *testing.T) {
	item := wellFormedSandbox("api")
	item.Ready.Timeout = -1
	set := Sandboxes{Sandboxes: []Sandbox{item}}
	findings := set.Validate()
	if len(findings) != 1 || findings[0].Code != "sandbox-ready-timeout-invalid" {
		t.Fatalf("Validate(negative timeout) = %+v, want exactly one sandbox-ready-timeout-invalid", findings)
	}
}

func TestSandboxesValidateAcceptsAWellFormedTimeout(t *testing.T) {
	item := wellFormedSandbox("api")
	item.Ready.Timeout = 90
	set := Sandboxes{Sandboxes: []Sandbox{item}}
	if findings := set.Validate(); len(findings) != 0 {
		t.Errorf("a well-formed timeout was rejected: %+v", findings)
	}
}

func TestSandboxesValidateReportsAnInvalidLogsFormat(t *testing.T) {
	item := wellFormedSandbox("api")
	item.Logs.Format = "xml"
	set := Sandboxes{Sandboxes: []Sandbox{item}}
	findings := set.Validate()
	if len(findings) != 1 || findings[0].Code != "sandbox-logs-format-invalid" {
		t.Fatalf("Validate(bad logs.format) = %+v, want exactly one sandbox-logs-format-invalid", findings)
	}
}

func TestSandboxesValidateAcceptsEachKnownLogsFormat(t *testing.T) {
	for _, format := range LogsFormats {
		item := wellFormedSandbox("api")
		item.Logs.Format = format
		set := Sandboxes{Sandboxes: []Sandbox{item}}
		if findings := set.Validate(); len(findings) != 0 {
			t.Errorf("logs.format %q was rejected: %+v", format, findings)
		}
	}
}

func TestSandboxesValidateReportsNoLogsSource(t *testing.T) {
	item := wellFormedSandbox("api")
	item.Logs.Source = ""
	set := Sandboxes{Sandboxes: []Sandbox{item}}
	findings := set.Validate()
	if len(findings) != 1 || findings[0].Code != "sandbox-logs-missing" {
		t.Fatalf("Validate(no logs.source) = %+v, want exactly one sandbox-logs-missing", findings)
	}
}

func TestSandboxesValidateReportsNoStop(t *testing.T) {
	item := wellFormedSandbox("api")
	item.Stop = ""
	set := Sandboxes{Sandboxes: []Sandbox{item}}
	findings := set.Validate()
	if len(findings) != 1 || findings[0].Code != "sandbox-stop-missing" {
		t.Fatalf("Validate(no stop) = %+v, want exactly one sandbox-stop-missing", findings)
	}
}
