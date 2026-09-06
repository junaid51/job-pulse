package main

import "strings"

import "testing"

func TestSlugVariantsReachTheRealBoards(t *testing.T) {
	// Every one of these is a board this app watches, and the left-hand side is
	// how the employer appears in a posting.
	cases := []struct{ employer, wanted string }{
		{"Lean Technologies", "leantech"},
		{"Namshi", "namshi"},
		{"Property Finder", "propertyfinder"},
		{"Al Jomaih Energy and Water", "al-jomaih-energy-water"},
		{"Sealy Mattress Middle East", "sealy-mattress-middle-east"},
		{"Halian | Managed Services, Recruitment Agency & Contract Staffing", "halian"},
		{"Bespin Global MEA, an e& enterprise company", "bespinglobalmea"},
		{"Quik Hire Staffing", "quikhirestaffing"},
	}
	for _, c := range cases {
		got := slugVariants(c.employer)
		found := false
		for _, v := range got {
			if v == c.wanted {
				found = true
			}
		}
		if !found {
			t.Errorf("slugVariants(%q) = %v, missing %q", c.employer, got, c.wanted)
		}
	}
}

func TestSlugVariantsStayASweepNotABruteForce(t *testing.T) {
	got := slugVariants("Some Very Long Employer Name International Holdings Limited")
	if len(got) > 6 {
		t.Errorf("produced %d variants: %v", len(got), got)
	}
	for _, v := range got {
		if strings.Contains(v, " ") {
			t.Errorf("variant %q has a space in it", v)
		}
	}
}
