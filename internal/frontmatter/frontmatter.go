// Package frontmatter reads and writes the YAML block at the top of every file
// in a store.
//
// It is hand-rolled rather than a YAML library because the format here is a
// handful of known keys holding scalars and one string list. That narrowness is
// what makes rejecting everything else safe — and rejecting is the point: a
// silently misparsed status is worse than a parse error.
package frontmatter

import (
	"fmt"
	"strings"
)

// Delimiter opens and closes the block.
const Delimiter = "---"

// Document is a parsed file: its frontmatter values and everything after them.
type Document struct {
	// Values holds a string or a []string per key, matching what the source
	// declared.
	Values map[string]any
	Body   string
}

// Field is one key on its way out. Rendering takes an ordered slice rather than
// a map so a rewrite never reorders fields — an edit that shuffles them turns
// every diff into noise.
type Field struct {
	Key  string
	List []string
	Text string
	// IsList distinguishes an empty list from an empty string.
	IsList bool
}

// Text builds a scalar field.
func Text(key, value string) Field { return Field{Key: key, Text: value} }

// List builds a list field.
func List(key string, values []string) Field {
	return Field{Key: key, List: values, IsList: true}
}

// Parse splits a file into its frontmatter and body. A file without a block is
// an error: every file in a store carries one, so its absence is a defect rather
// than a document with no fields.
func Parse(raw []byte) (Document, error) {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if !strings.HasPrefix(text, Delimiter+"\n") {
		return Document{}, fmt.Errorf("no frontmatter: the file must open with %q on its own line", Delimiter)
	}
	rest := text[len(Delimiter)+1:]
	end := strings.Index(rest, "\n"+Delimiter)
	if end < 0 {
		return Document{}, fmt.Errorf("unterminated frontmatter: no closing %q", Delimiter)
	}
	block := rest[:end]
	// The body is stored without its leading blank line and Render puts exactly
	// one back, so a parse-then-render round trip is byte-stable.
	body := strings.TrimLeft(rest[end+len(Delimiter)+1:], "\n")

	values, err := parseBlock(block)
	if err != nil {
		return Document{}, err
	}
	return Document{Values: values, Body: body}, nil
}

func parseBlock(block string) (map[string]any, error) {
	values := map[string]any{}
	lines := strings.Split(block, "\n")

	for index := 0; index < len(lines); index++ {
		line := lines[index]
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.HasPrefix(line, "\t") || strings.HasPrefix(line, " ") {
			return nil, fmt.Errorf("line %d: unexpected indentation — frontmatter here is flat keys, not nested structures", index+1)
		}
		key, raw, found := strings.Cut(line, ":")
		if !found {
			return nil, fmt.Errorf("line %d: expected `key: value`, got %q", index+1, line)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("line %d: empty key", index+1)
		}
		if _, exists := values[key]; exists {
			return nil, fmt.Errorf("line %d: %q appears more than once", index+1, key)
		}
		raw = strings.TrimSpace(raw)

		switch {
		case raw == "":
			items, consumed, err := parseBlockList(lines, index+1)
			if err != nil {
				return nil, err
			}
			values[key] = items
			index += consumed
		case strings.HasPrefix(raw, "["):
			items, err := parseInlineList(raw, index+1)
			if err != nil {
				return nil, err
			}
			values[key] = items
		default:
			scalar, err := unquote(raw, index+1)
			if err != nil {
				return nil, err
			}
			values[key] = scalar
		}
	}
	return values, nil
}

func parseBlockList(lines []string, start int) ([]string, int, error) {
	var items []string
	consumed := 0
	for index := start; index < len(lines); index++ {
		trimmed := strings.TrimSpace(lines[index])
		if trimmed == "" {
			consumed++
			continue
		}
		if !strings.HasPrefix(trimmed, "- ") {
			break
		}
		value, err := unquote(strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")), index+1)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, value)
		consumed++
	}
	if len(items) == 0 {
		return nil, 0, fmt.Errorf("line %d: a key with no value must be followed by a `- item` list", start)
	}
	return items, consumed, nil
}

func parseInlineList(raw string, line int) ([]string, error) {
	if !strings.HasSuffix(raw, "]") {
		return nil, fmt.Errorf("line %d: unterminated list, expected a closing `]`", line)
	}
	inner := strings.TrimSpace(raw[1 : len(raw)-1])
	if inner == "" {
		return []string{}, nil
	}
	var items []string
	for _, part := range strings.Split(inner, ",") {
		value, err := unquote(strings.TrimSpace(part), line)
		if err != nil {
			return nil, err
		}
		if value == "" {
			return nil, fmt.Errorf("line %d: empty item in list", line)
		}
		items = append(items, value)
	}
	return items, nil
}

func unquote(raw string, line int) (string, error) {
	if len(raw) >= 2 {
		first, last := raw[0], raw[len(raw)-1]
		if first == '"' && last == '"' {
			return strings.ReplaceAll(raw[1:len(raw)-1], `\"`, `"`), nil
		}
		if first == '\'' && last == '\'' {
			return raw[1 : len(raw)-1], nil
		}
		if first == '"' || first == '\'' {
			return "", fmt.Errorf("line %d: unterminated quote", line)
		}
	}
	if strings.ContainsAny(raw, "{}&*|>") {
		return "", fmt.Errorf("line %d: %q uses YAML syntax this format does not support — quote it if it is literal text", line, raw)
	}
	return raw, nil
}

// String reads a scalar. It reports false for a key that is absent or holds a
// list, so a caller never silently takes the wrong shape.
func (d Document) String(key string) (string, bool) {
	value, ok := d.Values[key].(string)
	return value, ok
}

// Strings reads a list.
func (d Document) Strings(key string) ([]string, bool) {
	value, ok := d.Values[key].([]string)
	return value, ok
}

// Has reports whether a key was declared at all, whatever its shape.
func (d Document) Has(key string) bool {
	_, ok := d.Values[key]
	return ok
}

// Keys the parser understood but a caller did not claim. Used to reject a field
// nobody reads rather than let it sit there looking meaningful.
func (d Document) Unknown(known ...string) []string {
	allowed := make(map[string]bool, len(known))
	for _, key := range known {
		allowed[key] = true
	}
	var unknown []string
	for key := range d.Values {
		if !allowed[key] {
			unknown = append(unknown, key)
		}
	}
	return unknown
}

// Render writes a file back. Fields go out in the order given, and a body is
// separated by exactly one blank line.
func Render(fields []Field, body string) []byte {
	var out strings.Builder
	out.WriteString(Delimiter + "\n")
	for _, field := range fields {
		if field.IsList {
			out.WriteString(field.Key + ": [" + strings.Join(field.List, ", ") + "]\n")
			continue
		}
		out.WriteString(field.Key + ": " + quoteIfNeeded(field.Text) + "\n")
	}
	out.WriteString(Delimiter + "\n")

	body = strings.TrimLeft(body, "\n")
	if body != "" {
		out.WriteString("\n")
		out.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			out.WriteString("\n")
		}
	}
	return []byte(out.String())
}

// quoteIfNeeded quotes a value the parser would otherwise misread or reject.
func quoteIfNeeded(value string) string {
	if value == "" {
		return `""`
	}
	if strings.ContainsAny(value, ":#{}&*|>[]\"'") || strings.TrimSpace(value) != value {
		return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
	}
	return value
}
