package store

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/franwerner/openrecord/internal/finding"
	"github.com/franwerner/openrecord/internal/frontmatter"
)

// Status is what a record settles. Two values, and only two: there is no
// approval queue, a rejected option lives inside the record that rejected it,
// and superseding is an edit git already carries.
type Status string

const (
	// Accepted governs the code and is what work is verified against.
	Accepted Status = "accepted"
	// Pending means someone said explicitly that this is undecided, so it
	// constrains nothing.
	Pending Status = "pending"
)

// Frontmatter field names, in the order they are written. Order is fixed so a
// rewrite never reorders fields and turns every edit into a noisy diff.
const (
	FieldTitle       = "title"
	FieldDescription = "description"
	FieldStatus      = "status"
	FieldComponents  = "components"
)

// Index is what a level's INDEX.md carries — and all it carries. It never
// enumerates what is inside: listing is generated from frontmatter, so there is
// nothing to fall out of sync.
type Index struct {
	Title       string
	Description string
}

// Record is a decision record or a capability spec.
type Record struct {
	Title       string
	Description string
	Status      Status
	// Components is the surfaces a spec reaches. Mandatory there, absent on a
	// decision, which is closed by the component it is filed under.
	Components []string
	Body       string
}

// ReadIndex loads a level's index.
func ReadIndex(path string) (Index, []finding.Finding) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Index{}, []finding.Finding{finding.Errorf(finding.CodeIndexMissingFields,
			"cannot read the index: %v", err).At(relative(path))}
	}
	document, parseErr := frontmatter.Parse(raw)
	if parseErr != nil {
		return Index{}, []finding.Finding{finding.Errorf(finding.CodeInvalidFrontmatter,
			"%v", parseErr).At(relative(path))}
	}

	var findings []finding.Finding
	index := Index{}
	index.Title, _ = document.String(FieldTitle)
	index.Description, _ = document.String(FieldDescription)

	if strings.TrimSpace(index.Title) == "" || strings.TrimSpace(index.Description) == "" {
		findings = append(findings, finding.Errorf(finding.CodeIndexMissingFields,
			"an index needs a title and a description: the description is what tells a reader whether to descend here").At(relative(path)))
	}
	if unknown := document.Unknown(FieldTitle, FieldDescription); len(unknown) > 0 {
		findings = append(findings, finding.Errorf(finding.CodeInvalidFrontmatter,
			"an index carries only title and description, found %s", strings.Join(unknown, ", ")).At(relative(path)))
	}
	if strings.TrimSpace(document.Body) != "" {
		findings = append(findings, finding.Errorf(finding.CodeIndexHasBody,
			"an index carries no body — anything written here becomes a list, and a list is what generated listing replaces").At(relative(path)))
	}
	return index, findings
}

// ReadRecord loads a record. The kind decides which fields are required, so the
// caller says which store it came from rather than the file claiming it.
func ReadRecord(path string, kind Kind) (Record, []finding.Finding) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Record{}, []finding.Finding{finding.Errorf(finding.CodeInvalidFrontmatter,
			"cannot read the record: %v", err).At(relative(path))}
	}
	return ParseRecord(raw, kind, relative(path))
}

// ParseRecord validates a record's frontmatter without touching the disk, so the
// write path and validate share one implementation.
func ParseRecord(raw []byte, kind Kind, path string) (Record, []finding.Finding) {
	document, parseErr := frontmatter.Parse(raw)
	if parseErr != nil {
		return Record{}, []finding.Finding{finding.Errorf(finding.CodeInvalidFrontmatter,
			"%v", parseErr).At(path)}
	}

	var findings []finding.Finding
	record := Record{Body: document.Body}
	record.Title, _ = document.String(FieldTitle)
	record.Description, _ = document.String(FieldDescription)

	if strings.TrimSpace(record.Title) == "" {
		findings = append(findings, finding.Errorf(finding.CodeInvalidFrontmatter,
			"a record needs a title").At(path))
	}
	if strings.TrimSpace(record.Description) == "" {
		findings = append(findings, finding.Errorf(finding.CodeInvalidFrontmatter,
			"a record needs a description: it is what a reader sees when deciding whether to open the file, and no index lists anything else").At(path))
	}

	status, _ := document.String(FieldStatus)
	switch Status(status) {
	case Accepted, Pending:
		record.Status = Status(status)
	default:
		findings = append(findings, finding.Errorf(finding.CodeInvalidFrontmatter,
			"status is %q; the only values are %s and %s", status, Accepted, Pending).At(path))
	}

	known := []string{FieldTitle, FieldDescription, FieldStatus}
	if kind == Specs {
		known = append(known, FieldComponents)
		record.Components, _ = document.Strings(FieldComponents)
		if len(record.Components) == 0 {
			findings = append(findings, finding.Errorf(finding.CodeEmptyComponents,
				"a spec must declare the components it reaches: without them it crosses with no path and drops out of every scope check silently").At(path))
		}
	} else if document.Has(FieldComponents) {
		findings = append(findings, finding.Errorf(finding.CodeInvalidFrontmatter,
			"a decision does not declare components — it is closed by the one it is filed under").At(path))
	}

	if unknown := document.Unknown(known...); len(unknown) > 0 {
		findings = append(findings, finding.Errorf(finding.CodeInvalidFrontmatter,
			"unknown fields: %s", strings.Join(unknown, ", ")).At(path))
	}
	return record, findings
}

// RenderIndex writes an index back.
func RenderIndex(index Index) []byte {
	return frontmatter.Render([]frontmatter.Field{
		frontmatter.Text(FieldTitle, index.Title),
		frontmatter.Text(FieldDescription, index.Description),
	}, "")
}

// RenderRecord writes a record back, fields in their fixed order.
func RenderRecord(record Record, kind Kind) []byte {
	fields := []frontmatter.Field{
		frontmatter.Text(FieldTitle, record.Title),
		frontmatter.Text(FieldDescription, record.Description),
		frontmatter.Text(FieldStatus, string(record.Status)),
	}
	if kind == Specs {
		fields = append(fields, frontmatter.List(FieldComponents, record.Components))
	}
	return frontmatter.Render(fields, record.Body)
}

// relative trims a filesystem path back to something that reads like a
// coordinate, so a finding names the record rather than the machine it is on.
func relative(path string) string {
	slashed := filepath.ToSlash(path)
	if index := strings.LastIndex(slashed, "/"+Root+"/"); index >= 0 {
		return slashed[index+1:]
	}
	return slashed
}
