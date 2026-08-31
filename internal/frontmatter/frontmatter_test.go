package frontmatter

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseScalarsAndBody(t *testing.T) {
	doc, err := Parse([]byte("---\ntitle: Rate limiting\nstatus: accepted\n---\n\n## Context\n\nSomething.\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if title, _ := doc.String("title"); title != "Rate limiting" {
		t.Errorf("title = %q", title)
	}
	if !strings.HasPrefix(doc.Body, "## Context") {
		t.Errorf("body = %q", doc.Body)
	}
}

func TestParseLists(t *testing.T) {
	for name, source := range map[string]string{
		"inline": "---\ncomponents: [api, ui]\n---\n",
		"block":  "---\ncomponents:\n  - api\n  - ui\n---\n",
	} {
		t.Run(name, func(t *testing.T) {
			doc, err := Parse([]byte(source))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			got, ok := doc.Strings("components")
			if !ok {
				t.Fatal("components did not parse as a list")
			}
			if !reflect.DeepEqual(got, []string{"api", "ui"}) {
				t.Errorf("components = %v", got)
			}
		})
	}
}

func TestParseRejects(t *testing.T) {
	cases := map[string]string{
		"no frontmatter":  "# Just a heading\n",
		"unterminated":    "---\ntitle: x\n",
		"nested":          "---\ntitle: x\n  nested: y\n---\n",
		"no colon":        "---\njust a line\n---\n",
		"duplicate key":   "---\ntitle: a\ntitle: b\n---\n",
		"unterminated []": "---\ncomponents: [api, ui\n---\n",
		"open quote":      "---\ntitle: \"unclosed\n---\n",
		"yaml anchor":     "---\ntitle: &anchor\n---\n",
		"empty list item": "---\ncomponents: [api, , ui]\n---\n",
	}
	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(source)); err == nil {
				t.Fatalf("accepted invalid frontmatter: %q", source)
			}
		})
	}
}

func TestStringRejectsAListAndViceVersa(t *testing.T) {
	doc, err := Parse([]byte("---\ntitle: x\ncomponents: [api]\n---\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, ok := doc.String("components"); ok {
		t.Error("a list was read as a scalar")
	}
	if _, ok := doc.Strings("title"); ok {
		t.Error("a scalar was read as a list")
	}
}

func TestRoundTripIsStable(t *testing.T) {
	source := []byte("---\ntitle: Rate limiting at the gateway\ndescription: Applied at the gateway, not per handler.\nstatus: accepted\n---\n\n## Context\n\nSomething.\n")
	doc, err := Parse(source)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	title, _ := doc.String("title")
	description, _ := doc.String("description")
	status, _ := doc.String("status")

	rendered := Render([]Field{
		Text("title", title),
		Text("description", description),
		Text("status", status),
	}, doc.Body)

	if string(rendered) != string(source) {
		t.Errorf("round trip changed the file:\nwant %q\ngot  %q", source, rendered)
	}
}

func TestRenderQuotesWhatWouldNotParseBack(t *testing.T) {
	for _, value := range []string{
		"a value: with a colon",
		`a "quoted" word`,
		"[brackets]",
		"",
	} {
		rendered := Render([]Field{Text("title", value)}, "")
		doc, err := Parse(rendered)
		if err != nil {
			t.Fatalf("value %q rendered to something unparseable: %v (%s)", value, err, rendered)
		}
		if got, _ := doc.String("title"); got != value {
			t.Errorf("round trip of %q gave %q", value, got)
		}
	}
}

func TestUnknownReportsUnclaimedKeys(t *testing.T) {
	doc, err := Parse([]byte("---\ntitle: x\nsurprise: y\n---\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	unknown := doc.Unknown("title", "description", "status")
	if !reflect.DeepEqual(unknown, []string{"surprise"}) {
		t.Errorf("Unknown = %v", unknown)
	}
}
