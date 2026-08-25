// Package match decides whether a job satisfies a search profile.
package match

import (
	"strings"

	"github.com/junaid51/job-pulse/internal/providers"
)

// Criteria is the part of a search profile that matching cares about.
type Criteria struct {
	Keywords   []string
	Locations  []string
	RemoteOnly bool
}

// Matches reports whether j satisfies c.
//
// Keywords are matched case-insensitively against the title and locations
// against the location, each list OR-ed, an empty list meaning "anything".
//
// Substring matching is the entire strategy: no stemming, no fuzzy matching, no
// relevance score. If it turns out too loose or too strict in practice, this
// function is where that gets fixed.
func Matches(c Criteria, j providers.Job) bool {
	if c.RemoteOnly && !j.Remote {
		return false
	}
	if len(c.Locations) > 0 && !locationMatches(j.Location, c.Locations) {
		return false
	}
	positives, negatives := splitKeywords(c.Keywords)
	// Negatives stay literal: expanding "-frontend" through the alias
	// dictionary would silently exclude every "software engineer".
	if containsAnyFold(j.Title, negatives) {
		return false
	}
	if len(positives) > 0 {
		return keywordMatches(j.Title, positives)
	}
	// No positives left: an empty keyword list means "any title", and so does a
	// purely negative one — but a list of blanks keeps its old meaning of
	// matching nothing, so a stray space never turns into match-everything.
	return len(c.Keywords) == 0 || len(negatives) > 0
}

// splitKeywords separates excludes from includes: "designer, -senior" means
// designer jobs that are not senior. A minus prefix is the entire syntax.
func splitKeywords(keywords []string) (positives, negatives []string) {
	for _, keyword := range keywords {
		keyword = strings.TrimSpace(keyword)
		if rest, negated := strings.CutPrefix(keyword, "-"); negated {
			if rest != "" {
				negatives = append(negatives, rest)
			}
			continue
		}
		if keyword != "" {
			positives = append(positives, keyword)
		}
	}
	return positives, negatives
}

// keywordMatches is containsAnyFold plus the role-alias expansion: a keyword is
// tried literally first, then through its dictionary entry, so "frontend" also
// finds "React Engineer". See aliases.go.
func keywordMatches(title string, wanted []string) bool {
	title = strings.ToLower(title)
	for _, w := range wanted {
		w = strings.ToLower(strings.TrimSpace(w))
		if w == "" {
			continue
		}
		aliases := keywordAliases[w]
		if !shortKey(w, aliases) && strings.Contains(title, w) {
			return true
		}
		for _, alias := range aliases {
			if strings.Contains(title, alias) {
				return true
			}
		}
	}
	return false
}

// KeywordTerms expands one keyword into every string worth substring-matching —
// the term itself plus its role-dictionary entry. The search bar reuses it so
// typing "frontend" finds the jobs a profile keyed on "frontend" matched,
// instead of the strict subset whose title spells the word out.
func KeywordTerms(raw string) []string {
	needle := strings.ToLower(strings.TrimSpace(raw))
	if needle == "" {
		return nil
	}
	aliases := keywordAliases[needle]
	if shortKey(needle, aliases) {
		return aliases
	}
	return append([]string{needle}, aliases...)
}

// SearchTerms expands one typed word for the search bar: the word plus its role
// aliases, and the word plus its place aliases. Unlike KeywordTerms it always
// keeps the literal, even for a two-letter one — the search bar matches on word
// boundaries, so "qa" can no longer hide inside "Qatar", and dropping short
// literals there would only lose real hits.
func SearchTerms(raw string) (roles, places []string) {
	needle := strings.ToLower(strings.TrimSpace(raw))
	if needle == "" {
		return nil, nil
	}
	return append([]string{needle}, keywordAliases[needle]...),
		append([]string{needle}, locationAliases[needle]...)
}

// shortKey reports whether an abbreviation should be matched by its expansions
// alone. Two or three letters hide inside ordinary words — "pm" spells the
// middle of "development", "qa" the first half of "Qatar" — so for an
// abbreviation the dictionary already explains, the expansions are what the
// searcher meant and the literal is only noise. Each such entry therefore
// carries an anchored form of itself ("qa ", "tpm ") to keep the plain title
// reachable.
func shortKey(needle string, aliases []string) bool {
	return len(needle) <= 3 && len(aliases) > 0
}

// LocationTerms expands one location shorthand into every string worth
// substring-matching — the needle itself plus its dictionary entry. The API
// reuses it so a search filter behaves exactly like profile matching.
func LocationTerms(raw string) []string {
	needle := strings.ToLower(strings.TrimSpace(raw))
	if needle == "" {
		return nil
	}
	return append([]string{needle}, locationAliases[needle]...)
}

// locationMatches is containsAnyFold plus the alias expansion. Aliases apply to
// locations only: expanding keywords the same way would be surprising.
func locationMatches(location string, wanted []string) bool {
	location = strings.ToLower(location)
	for _, w := range wanted {
		w = strings.ToLower(strings.TrimSpace(w))
		if w == "" {
			continue
		}
		if strings.Contains(location, w) {
			return true
		}
		for _, alias := range locationAliases[w] {
			if strings.Contains(location, alias) {
				return true
			}
		}
	}
	return false
}

func containsAnyFold(haystack string, needles []string) bool {
	haystack = strings.ToLower(haystack)
	for _, n := range needles {
		n = strings.ToLower(strings.TrimSpace(n))
		if n != "" && strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}
