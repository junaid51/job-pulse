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
	// --- the Gulf: each country, and the region as one word ---
	"uae":     {"united arab emirates", "dubai", "abu dhabi", "sharjah", "ajman", "ras al khaimah"},
	"ksa":     {"saudi arabia", "saudi", "riyadh", "jeddah", "dammam", "khobar", "neom", "tabuk"},
	"saudi":   {"saudi arabia", "riyadh", "jeddah", "dammam", "khobar", "neom", "tabuk"},
	"qatar":   {"doha"},
	"kuwait":  {"kuwait city"},
	"bahrain": {"manama"},
	"oman":    {"muscat"},
	"gulf": {"united arab emirates", "dubai", "abu dhabi", "sharjah", "saudi",
		"riyadh", "jeddah", "dammam", "khobar", "qatar", "doha", "kuwait",
		"bahrain", "manama", "oman", "muscat"},

	// --- wider Middle East and Africa ---
	"egypt":        {"cairo", "alexandria", "giza"},
	"jordan":       {"amman"},
	"lebanon":      {"beirut"},
	"turkey":       {"istanbul", "ankara", "izmir"},
	"morocco":      {"casablanca", "rabat"},
	"tunisia":      {"tunis"},
	"nigeria":      {"lagos", "abuja"},
	"kenya":        {"nairobi"},
	"ghana":        {"accra"},
	"south africa": {"johannesburg", "cape town", "durban", "pretoria"},

	// --- south and southeast Asia ---
	// Indian boards write cities and states, not the country: Paytm says
	// "Noida, Uttar Pradesh", Deliveroo says "Hyderabad - Main Office".
	"india": {"bengaluru", "bangalore", "mumbai", "delhi", "gurgaon", "gurugram",
		"hyderabad", "pune", "chennai", "noida", "kolkata", "ahmedabad", "jaipur",
		"indore", "kochi", "coimbatore", "lucknow", "chandigarh", "karnataka",
		"maharashtra", "rajasthan", "haryana", "uttar pradesh", "tamil nadu",
		"telangana", "kerala", "gujarat", "west bengal", "punjab"},
	"pakistan":    {"karachi", "lahore", "islamabad", "rawalpindi"},
	"bangladesh":  {"dhaka"},
	"sri lanka":   {"colombo"},
	"nepal":       {"kathmandu"},
	"philippines": {"manila", "cebu", "makati", "taguig"},
	"indonesia":   {"jakarta", "bandung", "surabaya"},
	"malaysia":    {"kuala lumpur", "penang"},
	"thailand":    {"bangkok"},
	"vietnam":     {"hanoi", "ho chi minh", "saigon"},

	// --- east Asia and the Pacific ---
	"china":       {"beijing", "shanghai", "shenzhen", "guangzhou", "hangzhou"},
	"taiwan":      {"taipei"},
	"japan":       {"tokyo", "osaka", "kyoto", "fukuoka"},
	"korea":       {"south korea", "seoul", "busan"},
	"australia":   {"sydney", "melbourne", "brisbane", "perth", "adelaide", "canberra"},
	"new zealand": {"auckland", "wellington"},

	// --- Europe ---
	// "uk" earned its place the same way "uae" did: a real profile matched two
	// jobs while fifty said "United Kingdom" — which does not contain "uk".
	"uk": {"united kingdom", "london", "manchester", "birmingham", "edinburgh",
		"glasgow", "bristol", "leeds", "cambridge", "oxford", "belfast", "cardiff",
		"sheffield", "newcastle"},
	"england":     {"united kingdom", "london", "manchester", "birmingham", "bristol", "leeds"},
	"ireland":     {"dublin", "cork", "galway"},
	"germany":     {"berlin", "munich", "hamburg", "frankfurt", "cologne", "stuttgart", "dusseldorf", "düsseldorf", "leipzig"},
	"france":      {"paris", "lyon", "marseille", "toulouse", "bordeaux", "nantes", "lille"},
	"netherlands": {"amsterdam", "rotterdam", "utrecht", "eindhoven", "the hague"},
	"holland":     {"netherlands", "amsterdam", "rotterdam", "utrecht"},
	"belgium":     {"brussels", "antwerp", "ghent"},
	"spain":       {"madrid", "barcelona", "valencia", "seville", "malaga", "málaga"},
	"portugal":    {"lisbon", "porto"},
	"italy":       {"milan", "rome", "turin"},
	"switzerland": {"zurich", "zürich", "geneva", "lausanne", "basel", "bern"},
	"austria":     {"vienna"},
	"poland":      {"warsaw", "krakow", "kraków", "wroclaw", "wrocław", "gdansk", "gdańsk", "poznan", "poznań"},
	"czech":       {"czechia", "prague", "brno"},
	"czechia":     {"czech republic", "prague", "brno"},
	"romania":     {"bucharest", "cluj"},
	"greece":      {"athens", "thessaloniki"},
	"sweden":      {"stockholm", "gothenburg"},
	"denmark":     {"copenhagen"},
	"norway":      {"oslo"},
	"finland":     {"helsinki"},

	// --- the Americas ---
	// ", us" (with the comma) catches "Remote, US" without the false matches a
	// bare "us" would invite.
	"usa": {"united states", ", us", "new york", "san francisco", "seattle",
		"austin", "boston", "chicago", "los angeles", "denver", "atlanta",
		"miami", "dallas", "houston", "phoenix", "portland", "san diego",
		"san jose", "san mateo", "palo alto", "mountain view", "sunnyvale",
		"philadelphia", "minneapolis", "salt lake", "raleigh", "nashville",
		"charlotte", "pittsburgh", "washington, d"},
	"us":      {"united states", ", us"},
	"america": {"united states", ", us", "new york", "san francisco", "seattle", "austin", "boston", "chicago"},
	"canada": {"toronto", "vancouver", "montreal", "montréal", "ottawa",
		"calgary", "edmonton", "waterloo", "ontario", "quebec", "british columbia"},
	"mexico":    {"mexico city", "guadalajara", "monterrey", "cdmx"},
	"brazil":    {"sao paulo", "são paulo", "rio de janeiro", "belo horizonte", "curitiba"},
	"argentina": {"buenos aires"},
	"colombia":  {"bogota", "bogotá", "medellin", "medellín"},
	"chile":     {"santiago"},
	"peru":      {"lima"},
}
