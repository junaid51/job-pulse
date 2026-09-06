package main

import "strings"

// Turning an employer's name into the identifier it uses on a hiring system is
// a string transformation, not a feat of reasoning, and neither a 3B nor a 7B
// model would do it: asked for Lean Technologies, both probed
// "lean-technologies" on six systems and never once tried "leantech", which is
// the real board. So the sweep is code, and the model is left the two jobs it
// is actually good at — reading a name like
// "Halian | Managed Services, Recruitment Agency & Contract Staffing" and
// knowing the employer is called Halian, and looking at what came back and
// saying whether it is really them.
func slugVariants(employer string) []string {
	name := strings.ToLower(strings.TrimSpace(employer))
	// Boards are named after the company, not its legal wrapper or its tagline.
	if cut, _, found := strings.Cut(name, "|"); found {
		name = cut
	}
	if cut, _, found := strings.Cut(name, ","); found {
		name = cut
	}
	cleaned := make([]rune, 0, len(name))
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			cleaned = append(cleaned, r)
		case r == ' ', r == '-', r == '_', r == '.', r == '&', r == '\'':
			cleaned = append(cleaned, ' ')
		}
	}
	words := strings.Fields(string(cleaned))
	// Words that are part of a company's name on paper and never in its slug.
	noise := map[string]bool{
		"the": true, "and": true, "llc": true, "ltd": true, "limited": true,
		"inc": true, "plc": true, "co": true, "company": true, "corp": true,
		"corporation": true, "gmbh": true, "bv": true, "sa": true, "fz": true,
		"fze": true, "llp": true, "group": true, "holding": true, "holdings": true,
	}
	// Long words a company shortens in its own handle.
	short := map[string]string{
		"technologies": "tech", "technology": "tech", "solutions": "solutions",
		"international": "intl", "systems": "systems", "digital": "digital",
	}

	kept := make([]string, 0, len(words))
	for _, w := range words {
		if !noise[w] {
			kept = append(kept, w)
		}
	}
	if len(kept) == 0 {
		kept = words
	}

	var out []string
	add := func(candidate string) {
		if candidate == "" || len(candidate) < 2 {
			return
		}
		for _, seen := range out {
			if seen == candidate {
				return
			}
		}
		out = append(out, candidate)
	}

	add(strings.Join(kept, ""))
	add(strings.Join(kept, "-"))
	// The abbreviated form: "lean technologies" -> "leantech".
	abbreviated := make([]string, len(kept))
	for i, w := range kept {
		if s, ok := short[w]; ok {
			abbreviated[i] = s
		} else {
			abbreviated[i] = w
		}
	}
	add(strings.Join(abbreviated, ""))
	add(strings.Join(abbreviated, "-"))
	if len(kept) > 1 {
		add(kept[0])                              // "namshi" from "namshi group"
		add(strings.Join(kept[:len(kept)-1], "")) // drop a trailing descriptor
	}
	if len(out) > 6 {
		out = out[:6] // a sweep, not a brute force
	}
	return out
}
