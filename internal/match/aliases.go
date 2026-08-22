// The alias dictionaries — the one file to edit when a search misses jobs it
// should have found. Both work the same way: the user's term is tried as a
// substring first, then each of its aliases. Case-insensitive throughout.
package match

// keywordAliases expands common role names into the words job titles actually
// use: "frontend" should find "React Engineer", because that is the same job
// wearing a different title. Deliberately a small curated dictionary: no AI,
// no embeddings, no fuzzy matching — an entry earns its place when a real
// search misses real jobs.
//
// Every alias must be a title a person searching the key would actually want.
// Generic titles are therefore banned: "frontend" once expanded to bare
// "software engineer", which handed a frontend search every backend, data and
// embedded job in the corpus. A generic title is still reachable — type it.
//
// Substring matching does the stemming for free: "react" also matches
// "reactjs", "node" matches "nodejs". Short aliases cut both ways — "java"
// also matches "javascript" — which is the accepted cost of keeping this a
// dictionary instead of a language model.
var keywordAliases = map[string][]string{
	"frontend": {
		"front-end", "front end", "front‑end",
		"react", "angular", "vue", "svelte", "nextjs", "next.js",
		"javascript", "typescript",
		"web developer", "web engineer", "ui engineer", "ui developer",
	},
	"backend": {
		"back-end", "back end",
		"golang", "java", "spring", "node", "python", "django", "rails",
		"api engineer", "api developer",
	},
	"full stack": {"full-stack", "fullstack", "mern", "mean stack"},
	"full-stack": {"full stack", "fullstack", "mern", "mean stack"},
	"fullstack":  {"full stack", "full-stack", "mern", "mean stack"},
	"mobile": {
		// "ios" cannot ride alone: as a substring it also spells Kiosk,
		// Studios and Radios. A leading space or a following noun anchors it.
		" ios", "ios developer", "ios engineer", "ios app",
		"android", "react native", "flutter", "swift", "kotlin",
	},
	"devops": {
		"sre", "site reliability", "platform engineer", "kubernetes",
		"infrastructure engineer", "cloud engineer", "devsecops", "gitops",
	},
	// Developer experience is its own discipline and the app understood none
	// of its names until someone doing the job said so.
	"dev ex": {
		"developer experience", "devex", "developer productivity",
		"developer platform", "internal tools", "platform engineer",
	},
	"devex": {
		"developer experience", "dev ex", "developer productivity",
		"developer platform", "internal tools", "platform engineer",
	},
	"developer experience": {
		"devex", "dev ex", "developer productivity", "developer platform",
		"internal tools", "platform engineer",
	},
	"data": {
		"data engineer", "data scientist", "analytics engineer",
		"machine learning", "ml engineer",
	},
	"design": {
		"designer", "ui/ux", "ux", "product designer", "graphic designer",
	},
	"product": {
		"product manager", "product owner", "technical product manager",
		"product lead", "head of product", "group product manager",
	},
	// Two letters, so the aliases carry it alone (see shortKey): the literal
	// "qa" also spells the first half of Qatar.
	"qa": {
		"qa ", "qa engineer", "qa analyst", "qa lead", "quality engineer",
		"quality assurance", "test engineer", "sdet", "automation engineer",
		"tester",
	},

	// --- the rest of the company ---
	// Engineering had a dictionary and nothing else did, so every
	// non-engineering search fell back to one literal substring and came back
	// nearly empty.
	"business analyst": {
		"business systems analyst", "systems analyst", "business intelligence",
		"bi analyst", "data analyst", "product owner", "functional consultant",
		"process analyst", "reporting analyst", "business analysis",
	},
	"analyst": {
		"analytics", "business intelligence", "bi analyst", "reporting analyst",
	},
	"project manager": {
		"program manager", "programme manager", "delivery manager", "pmo",
		"scrum master", "project management",
	},

	// --- engineering's own management track ---
	// The same shape as the analyst family: real titles for one job, not a
	// widening into every job.
	"program manager": {
		"programme manager", "technical program manager",
		"technical project manager", "delivery manager", "project manager",
	},
	"technical program manager": {
		"technical project manager", "program manager", "tpm ",
	},
	"delivery manager": {"delivery lead", "program manager", "project manager"},
	"scrum master":     {"agile coach", "agile delivery", "iteration manager"},
	"agile":            {"scrum master", "agile coach", "agile delivery", "kanban"},
	"engineering manager": {
		"head of engineering", "director of engineering",
		// Anchored to software: a bare "development manager" is a Business
		// Development Manager four times out of five, which is sales.
		"software development manager", "software engineering manager",
		"tech lead manager", "engineering lead",
	},
	"product owner": {
		"technical product owner", "product manager", "product management",
	},
	// Also two letters, and the literal spells the middle of "development".
	"pm": {
		"product manager", "program manager", "project manager",
		"programme manager", "delivery manager", "technical program manager",
	},
	"tpm": {"technical program manager", "technical project manager"},
	"finance": {
		"accountant", "accounting", "financial analyst", "controller",
		"treasury", "audit", "fp&a",
	},
	"marketing": {
		"brand manager", "growth manager", "digital marketing",
		"content marketing", "social media", "seo",
	},
	"human resources": {
		"talent acquisition", "recruiter", "people partner", "hrbp",
		"compensation", "learning and development",
	},
	"operations": {
		"supply chain", "logistics", "operations manager", "process improvement",
	},
	"sales": {
		"account executive", "account manager", "business development",
		"partnerships", "sales manager",
	},
	"customer support": {
		"customer success", "support specialist", "client services",
		"customer service",
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
	// Remote feeds spell the restriction into the location: "United States",
	// "EMEA", "Worldwide". This is how someone filters for the unrestricted
	// ones instead of scrolling past jobs they cannot legally take.
	"worldwide": {"anywhere", "global", "remote - global", "no restriction"},
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
