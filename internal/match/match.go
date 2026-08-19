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
	return len(c.Keywords) == 0 || keywordMatches(j.Title, c.Keywords)
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
		if strings.Contains(title, w) {
			return true
		}
		for _, alias := range keywordAliases[w] {
			if strings.Contains(title, alias) {
				return true
			}
		}
	}
	return false
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
