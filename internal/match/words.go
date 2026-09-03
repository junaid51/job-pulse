package match

import (
	"strings"
	"unicode"
)

// containsWord is strings.Contains with word edges, which is the difference
// between a place name and an accident. Plain substring matching put 250-odd
// Indiana jobs behind the "Gulf + India" filter, matched Ukraine for "uk", and
// notified a "Devops · Gulf" search about roles in Romania — "gulf" expands to
// "oman", and R-oman-ia contains it.
//
// A boundary is only required where the needle's own edge is a word character:
// "c++" and "node.js" end in punctuation and must still match "C++ Developer".
func containsWord(haystack, needle string) bool {
	if needle == "" {
		return false
	}
	first, last := rune(needle[0]), rune(needle[len(needle)-1])
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] != needle {
			continue
		}
		if wordish(first) && i > 0 && wordish(rune(haystack[i-1])) {
			continue
		}
		end := i + len(needle)
		// A plural is the same word. "platform" has to keep finding "Cloud
		// Security Platforms", which is a real posting; letting -ing or -er in
		// too is what put "Production Editor" in a search for "product", so the
		// tolerance stops at s and es.
		for _, plural := range []string{"es", "s"} {
			if strings.HasPrefix(strings.ToLower(haystack[end:]), plural) {
				end += len(plural)
				break
			}
		}
		if wordish(last) && end < len(haystack) && wordish(rune(haystack[end])) {
			continue
		}
		return true
	}
	return false
}

func wordish(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}
