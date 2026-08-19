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
// relevance score. If it turns out too loose in practice, this function is where
// that gets fixed.
func Matches(c Criteria, j providers.Job) bool {
	if c.RemoteOnly && !j.Remote {
		return false
	}
	if len(c.Locations) > 0 && !containsAnyFold(j.Location, c.Locations) {
		return false
	}
	return len(c.Keywords) == 0 || containsAnyFold(j.Title, c.Keywords)
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
