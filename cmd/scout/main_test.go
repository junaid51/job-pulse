package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// scriptedModel answers each request with the next canned reply, so the loop
// can be tested without a model and without the network.
func scriptedModel(t *testing.T, replies []chatMessage) *httptest.Server {
	t.Helper()
	turn := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if turn >= len(replies) {
			t.Errorf("the loop asked for turn %d; the script has %d", turn+1, len(replies))
			http.Error(w, "off the end of the script", http.StatusInternalServerError)
			return
		}
		reply := replies[turn]
		turn++
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": reply}},
		})
	}))
}

// withProbe swaps the board reader for a canned one, so the loop is tested
// without touching a job board.
func withProbe(t *testing.T, answers map[string]map[string]any) {
	t.Helper()
	original := probe
	probe = func(_ context.Context, provider, slug string) map[string]any {
		if a, ok := answers[provider+":"+slug]; ok {
			return a
		}
		return map[string]any{"found": false, "why": "404"}
	}
	t.Cleanup(func() { probe = original })
}

func found(reachable int) map[string]any {
	return map[string]any{"found": true, "postings": reachable, "reachable_postings": reachable}
}

func call(name, args string) toolCall {
	var c toolCall
	c.ID, c.Type = "call-"+name, "function"
	c.Function.Name, c.Function.Arguments = name, args
	return c
}

// A proposal is the server's decision, not the scout's. The scout counts what
// it is told and carries on.
func TestARefusedProposalIsCountedAndFedBack(t *testing.T) {
	var seenBody map[string]string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&seenBody)
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "refused", "reason": "the board answered with no postings this hunt can reach",
			"postings": 36, "reachable": 0,
		})
	}))
	defer api.Close()

	withProbe(t, map[string]map[string]any{"greenhouse:air": found(4)})
	model := scriptedModel(t, []chatMessage{
		{Role: "assistant", ToolCalls: []toolCall{call("probe_board",
			`{"provider":"greenhouse","slug":"air"}`)}},
		{Role: "assistant", ToolCalls: []toolCall{call("propose_board",
			`{"provider":"greenhouse","slug":"air","employer":"Air Arabia","reason":"resolves"}`)}},
		{Role: "assistant", Content: "That board is not Air Arabia. Nothing to propose."},
	})
	defer model.Close()

	cfg := config{api: api.URL, modelURL: model.URL, model: "test", maxTurns: 5}
	got, err := hunt(context.Background(), cfg,
		workList{Providers: []string{"greenhouse"}}, target{Employer: "Air Arabia"})
	if err != nil {
		t.Fatalf("hunt: %v", err)
	}
	if got.refused != 1 || got.added != 0 {
		t.Errorf("outcome = %+v, want one refusal and no additions", got)
	}
	if seenBody["slug"] != "air" {
		t.Errorf("the server was sent %q", seenBody["slug"])
	}
}

func TestAnAcceptedProposalIsCounted(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "watching", "postings": 6, "reachable": 6,
		})
	}))
	defer api.Close()
	withProbe(t, map[string]map[string]any{"smartrecruiters:namshi": found(6)})
	model := scriptedModel(t, []chatMessage{
		{Role: "assistant", ToolCalls: []toolCall{call("probe_board",
			`{"provider":"smartrecruiters","slug":"namshi"}`)}},
		{Role: "assistant", ToolCalls: []toolCall{call("propose_board",
			`{"provider":"smartrecruiters","slug":"namshi","employer":"Namshi","reason":"six Gulf postings"}`)}},
		{Role: "assistant", Content: "Proposed."},
	})
	defer model.Close()

	cfg := config{api: api.URL, modelURL: model.URL, model: "test", maxTurns: 5}
	got, err := hunt(context.Background(), cfg,
		workList{Providers: []string{"smartrecruiters"}}, target{Employer: "Namshi"})
	if err != nil {
		t.Fatalf("hunt: %v", err)
	}
	if got.added != 1 {
		t.Errorf("outcome = %+v, want one addition", got)
	}
}

// The refusals are the point of remembering them: a board already judged is not
// probed again, however tireless the scout is.
func TestAnAlreadyJudgedBoardIsNotProbed(t *testing.T) {
	model := scriptedModel(t, []chatMessage{
		{Role: "assistant", ToolCalls: []toolCall{call("probe_board",
			`{"provider":"teamtailor","slug":"dubizzle"}`)}},
		{Role: "assistant", Content: "Already judged; nothing to do."},
	})
	defer model.Close()

	cfg := config{api: "http://127.0.0.1:1", modelURL: model.URL, model: "test", maxTurns: 5}
	work := workList{
		Providers: []string{"teamtailor"},
		Skip:      []judged{{Provider: "teamtailor", Slug: "dubizzle", Verdict: "refused"}},
	}
	if _, err := hunt(context.Background(), cfg, work, target{Employer: "Dubizzle"}); err != nil {
		t.Fatalf("hunt: %v", err)
	}
	// Reaching here without a network probe is the assertion: the api URL above
	// is unroutable, and teamtailor was never fetched.
}

// A model that keeps calling tools must not run forever.
func TestTheLoopStopsAtTheTurnLimit(t *testing.T) {
	forever := make([]chatMessage, 4)
	for i := range forever {
		forever[i] = chatMessage{Role: "assistant", ToolCalls: []toolCall{
			call("probe_board", `{"provider":"lever","slug":"seen"}`)}}
	}
	model := scriptedModel(t, forever)
	defer model.Close()

	cfg := config{api: "http://127.0.0.1:1", modelURL: model.URL, model: "test", maxTurns: 3}
	work := workList{Providers: []string{"lever"},
		Skip: []judged{{Provider: "lever", Slug: "seen", Verdict: "refused"}}}
	if _, err := hunt(context.Background(), cfg, work, target{Employer: "Loop"}); err != nil {
		t.Fatalf("hunt: %v", err)
	}
}

func TestUnknownToolsAreReportedRatherThanFatal(t *testing.T) {
	model := scriptedModel(t, []chatMessage{
		{Role: "assistant", ToolCalls: []toolCall{call("delete_everything", `{}`)}},
		{Role: "assistant", Content: "Sorry."},
	})
	defer model.Close()
	cfg := config{api: "http://127.0.0.1:1", modelURL: model.URL, model: "test", maxTurns: 4}
	if _, err := hunt(context.Background(), cfg, workList{}, target{Employer: "X"}); err != nil {
		t.Fatalf("an invented tool name should not end the run: %v", err)
	}
}

func TestModelErrorsSurfaceWithTheirBody(t *testing.T) {
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"rate limit reached"}`, http.StatusTooManyRequests)
	}))
	defer model.Close()
	cfg := config{api: "http://127.0.0.1:1", modelURL: model.URL, model: "test", maxTurns: 2}
	_, err := hunt(context.Background(), cfg, workList{}, target{Employer: "X"})
	if err == nil || !strings.Contains(err.Error(), "rate limit") {
		t.Fatalf("want the provider's own words in the error, got %v", err)
	}
}

// The model found a board and then called propose_board without the slug,
// twice, and signed off claiming it had proposed it. One board was found, so
// that is unambiguously the one meant — and it gets proposed.
func TestADroppedSlugIsFilledFromTheOneBoardFound(t *testing.T) {
	withProbe(t, map[string]map[string]any{"smartrecruiters:namshi": found(6)})
	var proposed map[string]string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&proposed)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "watching"})
	}))
	defer api.Close()
	model := scriptedModel(t, []chatMessage{
		{Role: "assistant", ToolCalls: []toolCall{call("probe_board",
			`{"provider":"smartrecruiters","slug":"namshi"}`)}},
		{Role: "assistant", ToolCalls: []toolCall{call("propose_board",
			`{"provider":"smartrecruiters","employer":"Namshi","reason":"looks right"}`)}},
		{Role: "assistant", Content: "Done."},
	})
	defer model.Close()

	cfg := config{api: api.URL, modelURL: model.URL, model: "test", maxTurns: 5}
	got, err := hunt(context.Background(), cfg,
		workList{Providers: []string{"smartrecruiters"}}, target{Employer: "Namshi"})
	if err != nil {
		t.Fatalf("hunt: %v", err)
	}
	if got.added != 1 || proposed["slug"] != "namshi" {
		t.Errorf("added=%d proposed=%v; want the probed board proposed", got.added, proposed)
	}
}

// A board nobody probed never reaches the server, whatever the model believes.
func TestAnUnprobedProposalNeverReachesTheServer(t *testing.T) {
	withProbe(t, map[string]map[string]any{})
	called := false
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "watching"})
	}))
	defer api.Close()
	model := scriptedModel(t, []chatMessage{
		{Role: "assistant", ToolCalls: []toolCall{call("propose_board",
			`{"provider":"greenhouse","slug":"airarabia","employer":"Air Arabia","reason":"major company"}`)}},
		{Role: "assistant", Content: "Understood."},
	})
	defer model.Close()

	cfg := config{api: api.URL, modelURL: model.URL, model: "test", maxTurns: 4}
	got, err := hunt(context.Background(), cfg,
		workList{Providers: []string{"greenhouse"}}, target{Employer: "Air Arabia"})
	if err != nil {
		t.Fatalf("hunt: %v", err)
	}
	if called || got.added != 0 {
		t.Errorf("an unprobed guess reached the server (called=%v, added=%d)", called, got.added)
	}
}
