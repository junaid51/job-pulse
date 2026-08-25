package api

import (
	"strings"
	"testing"
)

// The bug this replaced: the whole query was one substring pattern, so word
// order decided whether anything came back and a query spanning two fields
// never matched at all.
func TestSearchSQLRequiresEveryWord(t *testing.T) {
	var b binder
	sql := searchSQL([]string{"engineer", "dubai"}, &b)
	if strings.Count(sql, " and (") != 2 {
		t.Errorf("want one condition per word, got %q", sql)
	}
	if strings.Contains(sql, " or (j.title") {
		t.Error("words must be ANDed, not ORed")
	}
	// title patterns, location patterns and a company pattern for each word
	if len(b.args()) != 6 {
		t.Errorf("args = %d, want 6: %v", len(b.args()), b.args())
	}
}

func TestSearchWordsSplitsPlaceTokens(t *testing.T) {
	words, places := searchWords("senior react @dubai @  remote")
	if strings.Join(words, ",") != "senior,react,remote" {
		t.Errorf("words = %v", words)
	}
	if strings.Join(places, ",") != "dubai" {
		t.Errorf("places = %v", places)
	}
}

func TestBoundaryAnchorsOnlyWordCharacters(t *testing.T) {
	if got := boundary("react"); got != `\yreact\y` {
		t.Errorf("boundary(react) = %q", got)
	}
	// A plus sign is not a word character, so an anchor after it can never
	// match — "c++" would find nothing.
	if got := boundary("c++"); got != `\yc\+\+` {
		t.Errorf("boundary(c++) = %q", got)
	}
	if got := boundary(".net"); got != `\.net\y` {
		t.Errorf("boundary(.net) = %q", got)
	}
}

// "D4 Insight" is a real company and "d4" is two characters. A length rule that
// skipped short words for company names lost it entirely; the word boundaries
// are what keep "qa" out of "Qatar", so no length rule is needed.
func TestShortWordsStillMatchCompanyNames(t *testing.T) {
	var b binder
	searchSQL([]string{"d4"}, &b)
	if len(b.args()) != 3 {
		t.Errorf("args = %v, want title, location and company patterns", b.args())
	}
	if got := b.args()[2]; got != `\yd4\y` {
		t.Errorf("company pattern = %q", got)
	}
}
