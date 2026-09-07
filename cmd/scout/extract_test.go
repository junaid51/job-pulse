package main

import "testing"

// Every fixture here is a real fragment from a real careers page, and each one
// is a slug that name-guessing failed to produce.
func TestExtractionFindsWhatGuessingCannot(t *testing.T) {
	cases := []struct {
		name, page, pageURL, provider, slug string
	}{
		{
			// Five spellings of "Etihad" all answered with an empty board; the
			// identifier has a digit on the end.
			name:     "Etihad, on SmartRecruiters",
			page:     `<a href="https://careers.smartrecruiters.com/EtihadAirways5/744000143259659-cabin-crew">Cabin Crew</a>`,
			pageURL:  "https://careers.etihad.com",
			provider: "smartrecruiters", slug: "EtihadAirways5",
		},
		{
			name:     "Emaar, on an Oracle tenant",
			page:     `var url = "https://emhm.fa.em2.oraclecloud.com/hcmUI/CandidateExperience/en/sites/CX_1001/";`,
			pageURL:  "https://www.emaar.com/en/careers/",
			provider: "oracle", slug: "emhm.fa.em2.oraclecloud.com|CX_1001",
		},
		{
			name:     "a Workday host and the site on it",
			page:     `<iframe src="https://kbr.wd5.myworkdayjobs.com/en-US/KBR_Careers"></iframe>`,
			pageURL:  "https://www.kbr.com/careers",
			provider: "workday", slug: "kbr.wd5.myworkdayjobs.com/KBR_Careers",
		},
		{
			name:     "a Workday site named only in an API call",
			page:     `fetch("https://acme.wd3.myworkdayjobs.com/wday/cxs/acme/External/jobs")`,
			pageURL:  "https://acme.com/careers",
			provider: "workday", slug: "acme.wd3.myworkdayjobs.com/External",
		},
		{
			name:     "escaped inside JSON, as pages actually emit it",
			page:     `{"url":"https:\/\/jobs.smartrecruiters.com\/Namshi\/-retail-jobs"}`,
			pageURL:  "https://namshi.com/careers",
			provider: "smartrecruiters", slug: "Namshi",
		},
		{
			name:     "a Phenom-hosted site is named by its own host",
			page:     `<script src="/ph-static/app.js"></script>`,
			pageURL:  "https://careers.majidalfuttaim.com/global/en",
			provider: "phenom", slug: "careers.majidalfuttaim.com",
		},
	}
	for _, c := range cases {
		got := atsCandidates(c.page, c.pageURL)
		found := false
		for _, cand := range got {
			if cand.Provider == c.provider && cand.Slug == c.slug {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: wanted %s:%s, got %+v", c.name, c.provider, c.slug, got)
		}
	}
}

// A page mentioning a hiring system in passing should not turn into a board.
func TestExtractionIgnoresBoilerplate(t *testing.T) {
	page := `<a href="https://www.smartrecruiters.com/careers">We use SmartRecruiters</a>
	         <a href="https://jobs.lever.co/">Lever</a>
	         <a href="https://www.teamtailor.com/en/">Teamtailor</a>`
	for _, c := range atsCandidates(page, "https://example.com/about") {
		switch c.Slug {
		case "www", "careers", "jobs", "en":
			t.Errorf("boilerplate became a board: %+v", c)
		}
	}
}

func TestCareerLinksAreAbsoluteAndRelevant(t *testing.T) {
	page := `<a href="/en/careers/vacancies">Vacancies</a>
	         <a href="about-us">About</a>
	         <a href="https://external.test/jobs/search">Search openings</a>`
	got := careerLinks(page, "https://company.test/en/careers/")
	if len(got) != 2 {
		t.Fatalf("links = %v, want the two career ones", got)
	}
	for _, link := range got {
		if link[:5] != "https" {
			t.Errorf("link %q is not absolute", link)
		}
	}
}

func TestPrivateAddressesAreRefused(t *testing.T) {
	for _, host := range []string{"localhost", "127.0.0.1", "10.0.0.5", "192.168.1.1", "169.254.169.254"} {
		if !isPrivateHost(host) {
			t.Errorf("%s was treated as public", host)
		}
	}
	if isPrivateHost("careers.etihad.com") {
		t.Error("a real careers host was treated as private")
	}
}

// The sweep has to reach the pages that actually exist. careers.etihad.com is
// the one the model could not guess; ordering the shapes before the spellings
// is what puts it in the first three tried.
func TestCareersURLsReachTheRealPages(t *testing.T) {
	for _, c := range []struct{ employer, wanted string }{
		{"Etihad Airways", "https://careers.etihad.com"},
		{"Emaar", "https://www.emaar.com/en/careers"},
		{"Majid Al Futtaim", "https://careers.majidalfuttaim.com"},
	} {
		got := careersURLs(c.employer)
		found := false
		for _, u := range got {
			if u == c.wanted {
				found = true
			}
		}
		if !found {
			t.Errorf("careersURLs(%q) missing %q; got %v", c.employer, c.wanted, got)
		}
	}
}
