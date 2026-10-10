package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// withProbe swaps the board reader for a canned one, so a run is tested
// without touching a job board.
func withProbe(t *testing.T, boards map[string]probeResult) {
	t.Helper()
	original := probe
	probe = func(_ context.Context, provider, slug string) probeResult {
		if r, ok := boards[provider+":"+slug]; ok {
			return r
		}
		return probeResult{Why: "404"}
	}
	t.Cleanup(func() { probe = original })
}

// judgeServer is a model that answers "not the same" for any prompt naming one
// of the given boards, and "the same" otherwise. fail makes it answer 500.
func judgeServer(t *testing.T, namesakes []string, fail bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			http.Error(w, "model is down", http.StatusInternalServerError)
			return
		}
		body, _ := io.ReadAll(r.Body)
		same := true
		for _, n := range namesakes {
			if strings.Contains(string(body), "Candidate board: "+n) {
				same = false
			}
		}
		content := `<think>weighing it up</think>{"same_company": true, "reason": "the work matches"}`
		if !same {
			content = "```json\n{\"same_company\": false, \"reason\": \"a fitness app, not a data firm\"}\n```"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": content}}},
		})
	}))
}

// apiServer records what the scout sends the JobPulse server.
type apiServer struct {
	*httptest.Server
	mu        sync.Mutex
	proposals []map[string]string
	misses    []map[string]string
}

func newAPIServer(t *testing.T) *apiServer {
	t.Helper()
	a := &apiServer{}
	a.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		a.mu.Lock()
		defer a.mu.Unlock()
		switch r.URL.Path {
		case "/api/boards":
			a.proposals = append(a.proposals, in)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "watching"})
		case "/api/discovery/misses":
			a.misses = append(a.misses, in)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	return a
}

func newScout(model, api string) scout {
	return scout{
		cfg:  config{api: api, modelURL: model, model: "test", maxPages: 0},
		work: workList{Sweepable: []string{"greenhouse", "ashby"}, Providers: []string{"greenhouse", "ashby"}},
		skip: map[string]bool{},
	}
}

// "Future Data" sweeps to greenhouse:future, a fitness app. The judge says no,
// so it is never proposed, and the real board is.
func TestOnlyAJudgedYesIsProposed(t *testing.T) {
	withProbe(t, map[string]probeResult{
		"greenhouse:future": {Found: true, Postings: 9, Reachable: 9, Titles: []string{"Health Coach"}, Places: []string{"Remote"}},
		"ashby:futuredata":  {Found: true, Postings: 4, Reachable: 3, Titles: []string{"Data Engineer"}, Places: []string{"Dubai"}},
	})
	model := judgeServer(t, []string{"greenhouse:future"}, false)
	defer model.Close()
	api := newAPIServer(t)
	defer api.Close()

	s := newScout(model.URL, api.URL)
	s.place(t.Context(), target{Employer: "Future Data", Known: []string{"Data Analyst — Dubai"}}, "")

	if len(api.proposals) != 1 || api.proposals[0]["slug"] != "futuredata" {
		t.Fatalf("proposals = %v, want only ashby:futuredata", api.proposals)
	}
	if s.added != 1 || s.rejected != 1 || len(api.misses) != 0 {
		t.Errorf("added=%d rejected=%d misses=%v, want 1, 1, none", s.added, s.rejected, api.misses)
	}
}

// Every candidate a namesake: nothing proposed, and the employer rests.
func TestAnEmployerWithOnlyNamesakesRests(t *testing.T) {
	withProbe(t, map[string]probeResult{
		"greenhouse:future": {Found: true, Postings: 9, Reachable: 9, Titles: []string{"Health Coach"}},
	})
	model := judgeServer(t, []string{"greenhouse:future"}, false)
	defer model.Close()
	api := newAPIServer(t)
	defer api.Close()

	s := newScout(model.URL, api.URL)
	s.place(t.Context(), target{Employer: "Future"}, "")
	if len(api.proposals) != 0 {
		t.Errorf("a namesake was proposed: %v", api.proposals)
	}
	if len(api.misses) != 1 || !strings.Contains(api.misses[0]["reason"], "greenhouse:future") {
		t.Errorf("misses = %v, want one naming the rejected board", api.misses)
	}
}

// A judge that cannot answer is not a yes, and not a miss: the question was
// never settled, so the employer stays on tomorrow's list.
func TestAFailedJudgeIsNeitherAYesNorAMiss(t *testing.T) {
	withProbe(t, map[string]probeResult{
		"ashby:analog": {Found: true, Postings: 22, Reachable: 22, Titles: []string{"Data Engineer"}},
	})
	model := judgeServer(t, nil, true)
	defer model.Close()
	api := newAPIServer(t)
	defer api.Close()

	s := newScout(model.URL, api.URL)
	s.place(t.Context(), target{Employer: "Analog"}, "")
	if len(api.proposals) != 0 || len(api.misses) != 0 {
		t.Errorf("proposals=%v misses=%v, want neither", api.proposals, api.misses)
	}
}

// A board judged before is not judged again.
func TestAnAlreadyJudgedBoardIsNotACandidate(t *testing.T) {
	withProbe(t, map[string]probeResult{
		"ashby:analog": {Found: true, Postings: 22, Reachable: 22},
	})
	s := newScout("", "")
	s.skip["ashby:analog"] = true
	if got := s.sweep(t.Context(), "Analog"); len(got) != 0 {
		t.Errorf("sweep offered %v, which was judged before", got)
	}
}

// A replacement for a dead board is proposed as one, and a dead board's
// employer never rests: it is retried while the board stays broken.
func TestAReplacementNamesTheBoardItReplaces(t *testing.T) {
	withProbe(t, map[string]probeResult{
		"ashby:clickhouse": {Found: true, Postings: 30, Reachable: 5},
	})
	model := judgeServer(t, nil, false)
	defer model.Close()
	api := newAPIServer(t)
	defer api.Close()

	s := newScout(model.URL, api.URL)
	s.place(t.Context(), target{Employer: "ClickHouse"}, "greenhouse:clickhouse")
	if len(api.proposals) != 1 || api.proposals[0]["replaces"] != "greenhouse:clickhouse" {
		t.Errorf("proposals = %v, want one replacing greenhouse:clickhouse", api.proposals)
	}
}

// Reasoning models think out loud and small ones wrap the answer; the verdict
// is the last object naming same_company, wherever it sits.
func TestVerdictsAreReadOutOfWhateverWrapsThem(t *testing.T) {
	cases := map[string]bool{
		`{"same_company": true, "reason": "r"}`:                                                       true,
		"<think>maybe {\"same_company\": true}</think>\n{\"same_company\": false, \"reason\": \"r\"}": false,
		"Sure.\n```json\n{\"same_company\": true, \"reason\": \"r\"}\n```":                            true,
		`first {"same_company": true} then {"same_company": false, "reason": "x"}`:                    false,
	}
	for answer, want := range cases {
		v, err := parseVerdict(answer)
		if err != nil || v.Same != want {
			t.Errorf("parseVerdict(%q) = %v, %v; want same=%v", answer, v, err, want)
		}
	}
	if _, err := parseVerdict("I think it is probably them."); err == nil {
		t.Error("an answer with no verdict should be an error, not a guess")
	}
}

// The judge sees what decides the question: who the employer is known to be,
// and what the board actually posts.
func TestThePromptCarriesTheEvidence(t *testing.T) {
	p := evidence{
		Employer: "Lean Technologies", Known: []string{"Staff DevOps Engineer — Riyadh"},
		Board: board{Provider: "ashby", Slug: "leantech", probeResult: probeResult{
			Postings: 3, Titles: []string{"Senior Compliance Manager"}, Places: []string{"Riyadh, Saudi Arabia"}}},
	}.prompt()
	for _, want := range []string{"Lean Technologies", "Staff DevOps Engineer — Riyadh", "ashby:leantech", "Senior Compliance Manager — Riyadh, Saudi Arabia"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q:\n%s", want, p)
		}
	}
}

// An employer searched and not placed is reported, so the work list can rest
// them: before this, the same unfindable names topped the list every day.
func TestAMissIsReportedToTheServer(t *testing.T) {
	api := newAPIServer(t)
	defer api.Close()
	recordMiss(t.Context(), config{api: api.URL, token: "t0k"}, "Amazon", "no board found")
	if len(api.misses) != 1 || api.misses[0]["employer"] != "Amazon" || api.misses[0]["reason"] != "no board found" {
		t.Errorf("misses = %v", api.misses)
	}
}
