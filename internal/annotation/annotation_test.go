package annotation

import (
	"reflect"
	"testing"
)

func TestFindBasicMention(t *testing.T) {
	got := Find("// @record:specs/flow/upload-file.md#successful-upload\n")
	want := []Mention{{Path: "specs/flow/upload-file.md", Anchor: "successful-upload", Line: 1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestFindWholeFileReferenceHasNoAnchor(t *testing.T) {
	got := Find("-- @record:decisions/api/structure/layers.md\n")
	if len(got) != 1 {
		t.Fatalf("got %d mentions, want 1: %+v", len(got), got)
	}
	if got[0].Path != "decisions/api/structure/layers.md" || got[0].Anchor != "" {
		t.Errorf("got %+v", got[0])
	}
}

func TestFindIgnoresWhatPrecedesTheMarker(t *testing.T) {
	// The marker is read from `@record:` onward; what precedes it on the
	// line — a language comment opener, prose, anything — is irrelevant.
	cases := []string{
		"# @record:specs/flow/x.md#a\n",
		"// @record:specs/flow/x.md#a\n",
		"-- @record:specs/flow/x.md#a\n",
		"/* @record:specs/flow/x.md#a */\n",
		"some prose, then @record:specs/flow/x.md#a\n",
	}
	for _, text := range cases {
		got := Find(text)
		if len(got) != 1 || got[0].Path != "specs/flow/x.md" || got[0].Anchor != "a" {
			t.Errorf("Find(%q) = %+v, want one mention specs/flow/x.md#a", text, got)
		}
	}
}

func TestFindTwoHashesOnOneLine(t *testing.T) {
	// The first `#` is the YAML/shell/Python comment marker, the second
	// separates the path from the anchor. The binary searches from
	// `@record:` onward, so what the first `#` belongs to never matters.
	got := Find("# @record:specs/flow/upload-file.md#file-too-large\n")
	if len(got) != 1 || got[0].Path != "specs/flow/upload-file.md" || got[0].Anchor != "file-too-large" {
		t.Fatalf("got %+v", got)
	}
}

func TestFindStopsAtTrailingPunctuation(t *testing.T) {
	cases := map[string]Mention{
		`// @record:specs/flow/x.md#archivo-grande.` + "\n":   {Path: "specs/flow/x.md", Anchor: "archivo-grande"},
		`// "@record:specs/flow/x.md#archivo-grande"` + "\n":  {Path: "specs/flow/x.md", Anchor: "archivo-grande"},
		`/* @record:specs/flow/x.md#archivo-grande */` + "\n": {Path: "specs/flow/x.md", Anchor: "archivo-grande"},
		`// @record:specs/flow/x.md#archivo-grande,` + "\n":   {Path: "specs/flow/x.md", Anchor: "archivo-grande"},
		`// @record:specs/flow/x.md#archivo-grande)` + "\n":   {Path: "specs/flow/x.md", Anchor: "archivo-grande"},
	}
	for text, want := range cases {
		got := Find(text)
		if len(got) != 1 {
			t.Fatalf("Find(%q) = %+v, want exactly one mention", text, got)
		}
		if got[0].Path != want.Path || got[0].Anchor != want.Anchor {
			t.Errorf("Find(%q) = %+v, want %+v", text, got[0], want)
		}
	}
}

func TestFindWholeFileReferenceStopsAtTrailingPunctuationToo(t *testing.T) {
	cases := []string{
		`// @record:decisions/api/structure/layers.md.` + "\n",
		`// "@record:decisions/api/structure/layers.md"` + "\n",
		`/* @record:decisions/api/structure/layers.md */` + "\n",
	}
	for _, text := range cases {
		got := Find(text)
		if len(got) != 1 || got[0].Path != "decisions/api/structure/layers.md" || got[0].Anchor != "" {
			t.Errorf("Find(%q) = %+v, want decisions/api/structure/layers.md with no anchor", text, got)
		}
	}
}

func TestFindSeveralMentionsInOneFile(t *testing.T) {
	text := "// @record:specs/flow/upload-file.md#successful-upload\n" +
		"func a() {}\n\n" +
		"// @record:specs/flow/upload-file.md#file-too-large\n" +
		"func b() {}\n"
	got := Find(text)
	if len(got) != 2 {
		t.Fatalf("got %d mentions, want 2: %+v", len(got), got)
	}
	if got[0].Anchor != "successful-upload" || got[0].Line != 1 {
		t.Errorf("first mention = %+v", got[0])
	}
	if got[1].Anchor != "file-too-large" || got[1].Line != 4 {
		t.Errorf("second mention = %+v", got[1])
	}
}

func TestFindLineNumberAfterSeveralLines(t *testing.T) {
	text := "line one\nline two\nline three\n// @record:specs/flow/x.md#a\n"
	got := Find(text)
	if len(got) != 1 || got[0].Line != 4 {
		t.Fatalf("got %+v, want line 4", got)
	}
}

func TestFindNoMentionIsEmpty(t *testing.T) {
	if got := Find("nothing to see here\n"); len(got) != 0 {
		t.Errorf("got %+v, want none", got)
	}
}

func TestLocationFormatsFileAndLine(t *testing.T) {
	m := Mention{File: "src/api/upload_test.go", Line: 42}
	if got, want := m.Location(), "src/api/upload_test.go:42"; got != want {
		t.Errorf("Location() = %q, want %q", got, want)
	}
}

func TestIncludedExcludesStoreAndMarkdown(t *testing.T) {
	cases := map[string]bool{
		"src/api/upload_test.go":      true,
		"README.md":                   false,
		"specs/flow/x.MD":             false,
		".openrecord/components.json": false,
		".openrecord":                 false,
		"src/openrecord-notes/x.go":   true, // only the store root itself, not a lookalike name
	}
	for path, want := range cases {
		if got := included(path); got != want {
			t.Errorf("included(%q) = %v, want %v", path, got, want)
		}
	}
}
