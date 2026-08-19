// The alias dictionaries — the one file to edit when a search misses jobs it
// should have found. Both work the same way: the user's term is tried as a
// substring first, then each of its aliases. Case-insensitive throughout.
package match

// keywordAliases expands common role names into the words job titles actually
// use. "Frontend" should find "React Engineer" and plain "Software Engineer",
// because those are the same job wearing different titles. Deliberately a
// small curated dictionary: no AI, no embeddings, no fuzzy matching — an entry
// earns its place when a real search misses real jobs.
//
// Substring matching does the stemming for free: "react" also matches
// "reactjs", "node" matches "nodejs". Short aliases cut both ways — "java"
// also matches "javascript" — which is the accepted cost of keeping this a
// dictionary instead of a language model.
var keywordAliases = map[string][]string{
	"frontend": {
		"front-end", "front end",
		"software engineer", "software developer",
		"react", "javascript", "typescript", "angular", "vue",
		"web developer",
	},
	"backend": {
		"back-end", "back end",
		"software engineer", "software developer",
		"golang", "java", "spring", "node", "python",
	},
	"full stack": {
		"full-stack", "fullstack",
		"software engineer", "software developer",
	},
	"full-stack": {
		"full stack", "fullstack",
		"software engineer", "software developer",
	},
	"fullstack": {
		"full stack", "full-stack",
		"software engineer", "software developer",
	},
}

// locationAliases expands the shorthand people type into the strings job boards
// actually write: "uae" has to find "Dubai" and "Dubai, United Arab Emirates".
// Every expansion must be long enough to be a safe substring ("us" would match
// Australia).
var locationAliases = map[string][]string{
	"uae":   {"united arab emirates", "dubai", "abu dhabi", "sharjah"},
	"ksa":   {"saudi arabia", "saudi", "riyadh", "jeddah"},
	"saudi": {"saudi arabia", "riyadh", "jeddah"},
	// "uk" earned its place the same way "uae" did: a real profile matched two
	// jobs while fifty said "United Kingdom" — which does not contain "uk".
	"uk":  {"united kingdom", "london"},
	"usa": {"united states", "new york"},
	"us":  {"united states"},
}
