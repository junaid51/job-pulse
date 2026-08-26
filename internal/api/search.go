package api

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/junaid51/job-pulse/internal/match"
)

// binder collects query arguments and hands back the placeholder for each one.
//
// The queries here used to carry hand-numbered $1..$10 placeholders, which is
// how the search bar ended up matching one long substring: adding a condition
// meant renumbering everything after it, so nobody did. Now a condition is
// written where it belongs and the numbering takes care of itself.
type binder struct{ vals []any }

func (b *binder) add(v any) string {
	b.vals = append(b.vals, v)
	return "$" + strconv.Itoa(len(b.vals))
}

func (b *binder) args() []any { return b.vals }

// searchWords splits a typed query into the words to match, the "@place" tokens
// that filter location instead, and the "-word" tokens that rule a posting out.
//
// Exclusions exist because a board can be worth reading and still bury what you
// came for: adding the Gulf's big engineering firms means "engineer" now returns
// resident engineers, HVAC and geotechnical work. A saved search has always been
// able to say -civil; the search bar could not, which left the reader nothing to
// do about it.
func searchWords(query string) (words, places, excluded []string) {
	for _, token := range strings.Fields(query) {
		if place := strings.TrimPrefix(token, "@"); place != token {
			if place != "" {
				places = append(places, place)
			}
			continue
		}
		if word := strings.TrimPrefix(token, "-"); word != token {
			if word != "" {
				excluded = append(excluded, word)
			}
			continue
		}
		words = append(words, token)
	}
	return words, places, excluded
}

// searchSQL is one condition per word, and every word has to match something.
//
// "engineer dubai" means both of those things to the person typing it, so it
// cannot be one substring: as one, word order decided whether anything came
// back ("frontend engineer" found eleven jobs, "engineer frontend" none),
// "ontend engine" matched the middle of words, and any query that spanned two
// fields — a role and a city — returned nothing at all.
//
// Each word is matched on word boundaries against the title (through the same
// role dictionary a saved search uses, so "frontend" still finds React), the
// location (through the place atlas, so "uae" still finds Dubai), or the
// company name. Short words are safe here precisely because of the boundaries:
// "qa" matches "QA Engineer" and not "Qatar".
func searchSQL(words, excluded []string, b *binder) string {
	var conditions []string
	for _, word := range excluded {
		// Ruled out wherever it appears, and through the same dictionaries: a
		// reader who excludes "civil" should not have to also exclude every
		// wording of it.
		roles, places := match.SearchTerms(word)
		conditions = append(conditions,
			"(j.title !~* all("+b.add(boundaries(roles))+")"+
				" and j.location !~* all("+b.add(boundaries(places))+")"+
				" and j.company !~* "+b.add(boundary(word))+")")
	}
	for _, word := range words {
		roles, places := match.SearchTerms(word)
		// Short words need no special handling here, and must not get any:
		// the boundaries already stop "qa" hiding inside "Qatar", while a
		// length rule would lose "D4 Insight" and every other short company.
		conditions = append(conditions,
			"(j.title ~* any("+b.add(boundaries(roles))+")"+
				" or j.location ~* any("+b.add(boundaries(places))+")"+
				" or j.company ~* "+b.add(boundary(word))+")")
	}
	if len(conditions) == 0 {
		return ""
	}
	return " and " + strings.Join(conditions, " and ")
}

// boundary wraps a term so it matches whole words. The anchors go on only where
// the term itself starts or ends with a word character: "\yc++\y" would never
// match anything, because there is no word boundary after a plus sign.
func boundary(term string) string {
	quoted := regexp.QuoteMeta(strings.ToLower(strings.TrimSpace(term)))
	if quoted == "" {
		return ""
	}
	if isWordByte(quoted[0]) {
		quoted = `\y` + quoted
	}
	if isWordByte(quoted[len(quoted)-1]) {
		quoted += `\y`
	}
	return quoted
}

func boundaries(terms []string) []string {
	out := make([]string, 0, len(terms))
	for _, term := range terms {
		if pattern := boundary(term); pattern != "" {
			out = append(out, pattern)
		}
	}
	return out
}

func isWordByte(c byte) bool {
	return c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
