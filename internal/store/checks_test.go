package store

import "testing"

func TestLoadChecksReportsAnUndeclaredStoreAsBenign(t *testing.T) {
	repo := t.TempDir()
	_, err := LoadChecks(repo)
	if err == nil {
		t.Fatal("LoadChecks on an empty repo returned no error")
	}
	if !IsChecksUndeclared(err) {
		t.Errorf("IsChecksUndeclared(%v) = false, want true", err)
	}
}

func TestChecksSaveAndLoadRoundTrip(t *testing.T) {
	repo := t.TempDir()
	set := Checks{Checks: []Check{
		{ID: "arch-layers", Command: "go-arch-lint check", JustifiedBy: "decisions/api/delivery/go-arch-lint.md",
			Enforces: []string{"decisions/api/structure"}},
	}}
	if err := set.Save(repo); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadChecks(repo)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Hash == "" {
		t.Error("Save did not stamp a hash")
	}
	if findings := VerifyChecks(repo, reloaded); len(findings) != 0 {
		t.Errorf("VerifyChecks rejected what Save wrote: %+v", findings)
	}
	if !reloaded.Has("arch-layers") || !reloaded.Has("ARCH-LAYERS") {
		t.Error("Has is not case-insensitive")
	}
	item, ok := reloaded.Get("arch-layers")
	if !ok || item.Command != "go-arch-lint check" {
		t.Errorf("Get returned %+v, ok=%v", item, ok)
	}
}

func TestChecksValidateReportsADuplicateID(t *testing.T) {
	set := Checks{Checks: []Check{
		{ID: "arch-layers", Command: "a", JustifiedBy: "decisions/api/delivery/x.md"},
		{ID: "Arch-Layers", Command: "b", JustifiedBy: "decisions/api/delivery/x.md"},
	}}
	findings := set.Validate()
	if len(findings) != 1 || findings[0].Code != "check-duplicate" {
		t.Fatalf("Validate(duplicate id) = %+v, want exactly one check-duplicate", findings)
	}
}

func TestChecksValidateReportsAMissingCommand(t *testing.T) {
	set := Checks{Checks: []Check{
		{ID: "no-command", JustifiedBy: "decisions/api/delivery/x.md"},
	}}
	findings := set.Validate()
	if len(findings) != 1 || findings[0].Code != "check-without-command" {
		t.Fatalf("Validate(no command) = %+v, want exactly one check-without-command", findings)
	}
}

func TestChecksValidateReportsAMissingJustifiedBy(t *testing.T) {
	set := Checks{Checks: []Check{
		{ID: "no-justification", Command: "go test ./..."},
	}}
	findings := set.Validate()
	if len(findings) != 1 || findings[0].Code != "check-justified-by-missing" {
		t.Fatalf("Validate(no justified-by) = %+v, want exactly one check-justified-by-missing", findings)
	}
}

func TestChecksValidateAcceptsAWellFormedDeclaration(t *testing.T) {
	set := Checks{Checks: []Check{
		{ID: "arch-layers", Command: "go-arch-lint check", JustifiedBy: "decisions/api/delivery/x.md",
			Enforces: []string{"decisions/api/structure"}},
		{ID: "lint", Command: "golangci-lint run {files}", JustifiedBy: "decisions/api/delivery/x.md"},
	}}
	if findings := set.Validate(); len(findings) != 0 {
		t.Errorf("a well-formed declaration was rejected: %+v", findings)
	}
}

func TestChecksIDsAreSorted(t *testing.T) {
	set := Checks{Checks: []Check{
		{ID: "lint", Command: "a", JustifiedBy: "x"},
		{ID: "arch-layers", Command: "b", JustifiedBy: "x"},
	}}
	ids := set.IDs()
	if len(ids) != 2 || ids[0] != "arch-layers" || ids[1] != "lint" {
		t.Errorf("IDs = %v, want sorted [arch-layers lint]", ids)
	}
}

func TestOnChange(t *testing.T) {
	cases := map[string]bool{
		"golangci-lint run {files}":   true,
		"go-arch-lint check":          false,
		"go test ./...":               false,
		"npx playwright test {files}": true,
	}
	for command, want := range cases {
		if got := OnChange(command); got != want {
			t.Errorf("OnChange(%q) = %v, want %v", command, got, want)
		}
	}
}
