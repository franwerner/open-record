package check

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// hyphenRun collapses the hyphens Slug produces for every non-alphanumeric
// rune, so "a  &  b" comes out "a-b" rather than "a-----b".
var hyphenRun = regexp.MustCompile(`-+`)

// Slug converts free text — a scenario name, one side of a lifecycle
// transition — into the anchor that names it:
//
//  1. Lowercase.
//  2. Strip accents and diacritics: decompose (NFD) and drop the combining
//     marks, so á→a, ñ→n, ü→u.
//  3. Anything that is not a letter or a digit becomes a hyphen.
//  4. Collapse repeated hyphens, then trim one from each end.
//
// "Archivo muy grande (más de 50 MB)" becomes
// "archivo-muy-grande-mas-de-50-mb".
//
// The anchor is never stored — every consumer recomputes it from the name — so
// this rule is part of every reference already written and must not drift.
// That is why accents go through Unicode decomposition rather than a
// hand-written table: canonical decompositions are frozen by Unicode's
// stability policy, while a table invites "completing" later and would move
// existing anchors each time it grew. A letter with no decomposition (ß, ø,
// ł) is kept as written.
//
// A name with no letters or digits at all slugs to the empty string — Slug
// does not reject that, it is simply the input that produced it; the caller
// (Body's anchor check, via anchorSources) is what turns an empty result into
// a finding.
func Slug(text string) string {
	var builder strings.Builder
	for _, letter := range norm.NFD.String(strings.ToLower(text)) {
		switch {
		case unicode.Is(unicode.Mn, letter):
			continue
		case unicode.IsLetter(letter) || unicode.IsDigit(letter):
			builder.WriteRune(letter)
		default:
			builder.WriteRune('-')
		}
	}
	return strings.Trim(hyphenRun.ReplaceAllString(builder.String(), "-"), "-")
}
