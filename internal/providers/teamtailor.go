package providers

import (
	"context"
	"strings"
	"time"
)

// teamtailorFeed is the shape of https://{slug}.teamtailor.com/jobs.json — a
// JSON Feed whose items carry a schema.org JobPosting under "_jobposting".
// Large boards paginate with next_url.
type teamtailorFeed struct {
	NextURL string `json:"next_url"`
	Items   []struct {
		ID            string `json:"id"`
		Title         string `json:"title"`
		URL           string `json:"url"`
		DatePublished string `json:"date_published"`
		JobPosting    struct {
			HiringOrganization struct {
				Name string `json:"name"`
			} `json:"hiringOrganization"`
			// Missing on some postings, and holds several places when a job is
			// offered in more than one city.
			JobLocation []struct {
				Address struct {
					Locality string `json:"addressLocality"`
					Country  string `json:"addressCountry"` // ISO code: "AE", "SA"
				} `json:"address"`
			} `json:"jobLocation"`
		} `json:"_jobposting"`
	} `json:"items"`
}

// teamtailorMaxPages bounds the next_url walk; ten pages is a thousand postings,
// far past any board this tool watches.
const teamtailorMaxPages = 10

func fetchTeamtailor(ctx context.Context, slug string) ([]Job, error) {
	var jobs []Job
	url := "https://" + slug + ".teamtailor.com/jobs.json"
	for range teamtailorMaxPages {
		var feed teamtailorFeed
		if err := getJSON(ctx, url, &feed); err != nil {
			return nil, err
		}
		jobs = append(jobs, feed.jobs()...)
		if feed.NextURL == "" {
			return jobs, nil
		}
		url = feed.NextURL
	}
	return jobs, nil
}

func (f teamtailorFeed) jobs() []Job {
	jobs := make([]Job, 0, len(f.Items))
	for _, item := range f.Items {
		var places []string
		for _, p := range item.JobPosting.JobLocation {
			places = append(places, joinNonEmpty(p.Address.Locality, countryName(p.Address.Country)))
		}
		location := strings.Join(places, " / ")
		jobs = append(jobs, Job{
			ExternalID: item.ID,
			Company:    item.JobPosting.HiringOrganization.Name,
			Title:      strings.TrimSpace(item.Title),
			Location:   location,
			// The feed has no remote flag; the location text is all there is.
			Remote:   looksRemote(location),
			URL:      item.URL,
			PostedAt: parseTime(time.RFC3339, item.DatePublished),
		})
	}
	return jobs
}

// countryName expands the ISO codes Teamtailor uses into the names people
// search for — "AE" is a useless and unsafe substring, "United Arab Emirates"
// is what a profile matches. Data, not logic: an unknown code passes through.
func countryName(code string) string {
	if name, ok := isoCountries[code]; ok {
		return name
	}
	return code
}

var isoCountries = map[string]string{
	"AE": "United Arab Emirates", "SA": "Saudi Arabia", "QA": "Qatar",
	"KW": "Kuwait", "BH": "Bahrain", "OM": "Oman", "JO": "Jordan",
	"EG": "Egypt", "MA": "Morocco", "TR": "Turkey", "PK": "Pakistan",
	"IN": "India", "ID": "Indonesia", "MY": "Malaysia", "SG": "Singapore",
	"PH": "Philippines", "TH": "Thailand", "VN": "Vietnam", "CN": "China",
	"JP": "Japan", "KR": "South Korea", "AU": "Australia", "NZ": "New Zealand",
	"US": "United States", "CA": "Canada", "MX": "Mexico", "BR": "Brazil",
	"GB": "United Kingdom", "IE": "Ireland", "FR": "France", "DE": "Germany",
	"NL": "Netherlands", "BE": "Belgium", "ES": "Spain", "PT": "Portugal",
	"IT": "Italy", "PL": "Poland", "SE": "Sweden", "NO": "Norway",
	"DK": "Denmark", "FI": "Finland", "CH": "Switzerland", "AT": "Austria",
}
