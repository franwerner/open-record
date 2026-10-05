package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	openrecord "github.com/franwerner/openrecord"
	"github.com/franwerner/openrecord/internal/finding"
)

func emitted(t *testing.T, dir string) map[string]string {
	t.Helper()
	found := map[string]string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name(), "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		found[entry.Name()] = string(raw)
	}
	return found
}

// A fresh emit is one variant now: every bundled skill, setup-search
// included, with no qmd:start/qmd:end marker left in any of them — qmd is
// required for search, so every skill applies to every project unconditionally.
func TestSkillsEmitIsOneVariantWithNoMarkers(t *testing.T) {
	repo := project(t)
	out := filepath.Join(t.TempDir(), "skills")
	mustRun(t, repo, "skills", "--emit", out)

	files := emitted(t, out)
	if len(files) == 0 {
		t.Fatal("nothing was emitted")
	}
	if _, present := files["openrecord-setup-search"]; !present {
		t.Error("openrecord-setup-search was not emitted — there is only one variant now, and it always includes it")
	}
	for name, contents := range files {
		if strings.Contains(contents, "qmd:start") || strings.Contains(contents, "qmd:end") {
			t.Errorf("%s still carries a qmd marker in the emitted output", name)
		}
	}
}

// Nothing strips anything any more: an emitted skill must be byte-identical
// to its embedded source.
func TestSkillsEmitIsByteIdenticalToTheSource(t *testing.T) {
	repo := project(t)
	out := filepath.Join(t.TempDir(), "skills")
	mustRun(t, repo, "skills", "--emit", out)

	files := emitted(t, out)
	for name, contents := range files {
		source, err := openrecord.Assets.ReadFile("skills/" + name + "/SKILL.md")
		if err != nil {
			t.Fatal(err)
		}
		if string(source) != contents {
			t.Errorf("%s was altered even though nothing is stripped any more", name)
		}
	}
}

// --with-qmd no longer exists: passing it is an ordinary unknown-flag usage
// failure, the same as any other removed flag.
func TestSkillsWithQmdIsAUsageFailure(t *testing.T) {
	repo := project(t)
	out := filepath.Join(t.TempDir(), "skills")
	code, _, stderr := runIn(t, repo, "skills", "--emit", out, "--with-qmd")
	if code == exitOK {
		t.Fatal("--with-qmd was accepted")
	}
	result := decode[finding.Finding](t, stderr)
	if result.Code != finding.CodeUsage {
		t.Errorf("code = %q, want %q", result.Code, finding.CodeUsage)
	}
}

// A manifest left by a build that still wrote the old variant distinction
// must still report its now-gone skill as removed, exactly like any other
// file this build no longer ships.
func TestSkillsReportsASkillFromAnOldManifestAsRemoved(t *testing.T) {
	repo := project(t)
	out := filepath.Join(t.TempDir(), "skills")

	old := `{"version":1,"emitter":"test","with_qmd":true,"entries":[{"path":"stale-skill/SKILL.md","hash":"deadbeef"}]}`
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, ".openrecord-emitted.json"), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(out, "stale-skill"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Content matching the manifest's stamped hash of "" is not needed here:
	// an on-disk file that does not exist at all is simply skipped by Plan,
	// so write one whose bytes are irrelevant to this assertion but whose
	// removal is what is being checked.
	if err := os.WriteFile(filepath.Join(out, "stale-skill", "SKILL.md"), []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	report := decode[skillsReport](t, mustRun(t, repo, "skills", "--emit", out, "--dry-run"))
	if report.Counts["removed"] == 0 && report.Counts["kept"] == 0 {
		t.Errorf("a skill named only in an old with_qmd manifest was neither removed nor kept: %+v", report)
	}
}

func TestSkillsNeedsATarget(t *testing.T) {
	repo := project(t)
	if code, _, _ := runIn(t, repo, "skills"); code == exitOK {
		t.Error("skills without --emit succeeded")
	}
}
