// Command scout looks for job boards JobPulse is not watching yet.
//
// Code finds, a model judges, the server decides.
//
// It began as an agent: a model with tools, left to find each employer's board
// itself. Measured over 89 hunts in October 2026, every board it ever added had
// come from the name sweep — plain code trying spellings against the hiring
// systems addressed by a company's name — and the model's own exploration had
// produced none: 79% of the careers pages it opened were addresses it had
// invented. It also took two and a half minutes an employer, so the daily run
// reached eight of a pool of 170. The same sweep without the model covered a
// hundred employers in three minutes.
//
// What the sweep cannot do is tell a company from its namesake. "Future Data"
// swept straight to greenhouse:future, a fitness app hiring health coaches.
// That is a judgment — does this board belong to this employer? — and it is the
// one job here a model is good at. So each candidate board goes to the model as
// a single question, with what the board posts and what the employer is known
// to post, and only a yes is proposed. The server then probes the board itself
// and decides, as it always has.
//
// It runs against any OpenAI-compatible chat endpoint: ollama on a laptop or a
// CI runner, or a free hosted tier with a key.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/junaid51/job-pulse/internal/match"
	"github.com/junaid51/job-pulse/internal/providers"
)

func main() {
	if err := run(); err != nil {
		slog.Error("scout failed", "error", err)
		os.Exit(1)
	}
}

type config struct {
	api      string
	token    string
	modelURL string
	model    string
	modelKey string
	// reasoning is sent as reasoning_effort when set. "none" matters on a CPU
	// runner: a reasoning model left to think took over five minutes a
	// question there, and the judgment is one sentence long.
	reasoning  string
	maxTargets int
	maxPages   int
}

func run() error {
	cfg := config{
		api:       env("JOBPULSE_API", "http://localhost:8091"),
		token:     os.Getenv("POLL_TOKEN"),
		modelURL:  env("SCOUT_MODEL_URL", "http://localhost:11434/v1/chat/completions"),
		model:     env("SCOUT_MODEL", "gemma4:12b"),
		modelKey:  os.Getenv("SCOUT_MODEL_KEY"),
		reasoning: os.Getenv("SCOUT_MODEL_REASONING"),
	}
	flag.IntVar(&cfg.maxTargets, "targets", 100, "how many employers to sweep")
	// Careers pages are the slow route — a dozen fetches an employer — and the
	// one that reaches tenant-addressed systems no spelling produces. Budgeted
	// separately so a big sweep stays a few minutes.
	flag.IntVar(&cfg.maxPages, "pages", 20, "how many employers to read careers pages for when the sweep misses")
	// -list needs no model at all, so a scheduled run can find out whether
	// there is anything to do before spending minutes downloading one.
	listOnly := flag.Bool("list", false, "print what there is to look for, and stop")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Minute)
	defer cancel()

	work, err := fetchWork(ctx, cfg)
	if err != nil {
		return fmt.Errorf("asking the server what is worth looking for: %w", err)
	}
	if *listOnly {
		for _, t := range work.Targets {
			slog.Info("worth looking for", "employer", t.Employer, "matches", t.Matches)
		}
		for _, a := range work.Ailing {
			slog.Info("board has stopped answering", "board", a.Provider+":"+a.Slug,
				"employer", a.Employer, "postings still held", a.PostingsHeld,
				"error", truncate(a.Error, 80))
		}
		fmt.Printf("targets=%d\n", len(work.Targets)+len(work.Ailing))
		return nil
	}

	s := scout{cfg: cfg, work: work, skip: map[string]bool{}, pages: cfg.maxPages}
	for _, j := range work.Skip {
		s.skip[j.Provider+":"+j.Slug] = true
	}
	slog.Info("scouting", "employers", len(work.Targets), "ailing boards", len(work.Ailing),
		"already judged", len(work.Skip), "model", cfg.model)

	// The broken ones first: a board that has stopped answering is losing
	// alerts now, where a new employer is only ever an improvement.
	for _, a := range work.Ailing {
		employer := a.Employer
		if employer == "" {
			employer = a.Slug
		}
		s.place(ctx, target{Employer: employer}, a.Provider+":"+a.Slug)
	}
	for i, t := range work.Targets {
		if i >= cfg.maxTargets {
			break
		}
		s.place(ctx, t, "")
	}
	slog.Info("scout finished", "boards added", s.added, "proposals refused", s.refused,
		"candidates judged", s.judged, "rejected by the judge", s.rejected)
	writeSummary(s, work)
	return nil
}

// scout carries one run's state: what has been judged before, the careers-page
// budget, and the tally.
type scout struct {
	cfg   config
	work  workList
	skip  map[string]bool
	pages int

	added, refused, judged, rejected int
	log                              []string // one line per employer, for the run summary
}

// place looks for one employer's board and proposes it if the judge agrees.
// replaces is set when the employer's previous board has stopped answering.
func (s *scout) place(ctx context.Context, t target, replaces string) {
	candidates := s.sweep(ctx, t.Employer)
	route := "name sweep"
	if len(candidates) == 0 && s.pages > 0 {
		s.pages--
		candidates = s.careersPages(ctx, t.Employer)
		route = "careers pages"
	}
	if len(candidates) == 0 {
		s.note(t, replaces, "no board found")
		return
	}

	var rejected []string
	for _, c := range candidates {
		v, err := judge(ctx, s.cfg, evidence{
			Employer: t.Employer, Known: t.Known, Replaces: replaces, Board: c,
		})
		s.judged++
		if err != nil {
			// A judge that cannot answer is not a yes. Not a miss either: the
			// question was never settled, so the employer stays on the list.
			slog.Warn("the judge could not answer", "employer", t.Employer,
				"board", c.key(), "error", err)
			s.log = append(s.log, fmt.Sprintf("%s: judge failed on %s", t.Employer, c.key()))
			return
		}
		slog.Info("judged", "employer", t.Employer, "board", c.key(), "same company", v.Same,
			"reason", truncate(v.Reason, 160), "route", route)
		if !v.Same {
			s.rejected++
			rejected = append(rejected, c.key())
			continue
		}
		reason := fmt.Sprintf("%d reachable of %d postings; judge: %s", c.Reachable, c.Postings, v.Reason)
		answer := proposeBoard(ctx, s.cfg, c.Provider, c.Slug, t.Employer, truncate(reason, 300), replaces)
		s.skip[c.key()] = true
		switch answer["status"] {
		case "watching":
			s.added++
			s.log = append(s.log, fmt.Sprintf("%s: added %s (%d reachable postings, via %s)",
				t.Employer, c.key(), c.Reachable, route))
			return
		case "refused":
			s.refused++
			rejected = append(rejected, c.key()+" (refused by the server)")
		default:
			slog.Warn("unexpected answer to a proposal", "board", c.key(), "answer", answer)
		}
	}
	s.note(t, replaces, "judged not theirs: "+strings.Join(rejected, ", "))
}

// note records an employer that was searched and not placed. Only employers
// rest; a broken board is retried while it stays broken.
func (s *scout) note(t target, replaces, why string) {
	s.log = append(s.log, t.Employer+": "+why)
	if replaces == "" {
		recordMiss(context.Background(), s.cfg, t.Employer, why)
	}
}

// sweep tries the employer's spellings on every name-addressed system and
// returns the boards worth judging: reachable postings, a slug that resembles
// the name, not judged before. Largest first, at most three.
func (s *scout) sweep(ctx context.Context, employer string) []board {
	var out []board
	for _, b := range findBoards(ctx, s.work.sweep(), employer) {
		if b.Reachable > 0 && resemblesEmployer(b.Provider, b.Slug, employer) && !s.skip[b.key()] {
			out = append(out, b)
		}
	}
	if len(out) > 3 {
		out = out[:3]
	}
	return out
}

// careersPages reads the employer's careers pages for a hiring system the
// sweep cannot reach, and probes what they name.
func (s *scout) careersPages(ctx context.Context, employer string) []board {
	var out []board
	for _, c := range findHiringSystem(ctx, employer) {
		if s.skip[c.Provider+":"+c.Slug] || !isProposable(s.work.Providers, c.Provider) {
			continue
		}
		p := probe(ctx, c.Provider, c.Slug)
		if p.Found && p.Reachable > 0 {
			out = append(out, board{Provider: c.Provider, Slug: c.Slug, probeResult: p})
		}
		if len(out) == 3 {
			break
		}
	}
	return out
}

func isProposable(providers []string, provider string) bool {
	for _, p := range providers {
		if p == provider {
			return true
		}
	}
	return false
}

// --- what the server knows ------------------------------------------------

type target struct {
	Employer string `json:"employer"`
	Matches  int    `json:"matches"`
	// Known is a few of the employer's postings as the aggregator carried
	// them, "title — location": the judge's picture of who this employer is.
	Known []string `json:"known"`
}

type judged struct {
	Provider string `json:"provider"`
	Slug     string `json:"slug"`
	Verdict  string `json:"verdict"`
}

// ailing is a board that has stopped answering. Worth chasing because the
// failure is silent: it 404s once a cycle, its postings age out over a
// fortnight, and the alerts it would have sent never arrive.
type ailing struct {
	Provider     string `json:"provider"`
	Slug         string `json:"slug"`
	Employer     string `json:"employer"`
	Error        string `json:"error"`
	PostingsHeld int    `json:"postings_still_held"`
}

// scoutBoard is a board discovery added and what it has delivered since, for
// the run summary.
type scoutBoard struct {
	Board     string    `json:"board"`
	Employer  string    `json:"employer"`
	AddedAt   time.Time `json:"added_at"`
	Active    bool      `json:"active"`
	Postings  int       `json:"postings"`
	InMarket  int       `json:"in_market"`
	Matched   int       `json:"matched"`
	Verdict   string    `json:"verdict"`
	Rationale string    `json:"reason"`
}

type workList struct {
	Targets []target `json:"targets"`
	Ailing  []ailing `json:"ailing"`
	Skip    []judged `json:"do_not_try"`
	// Providers is everything that may be proposed; Sweepable is the narrower
	// set a spelling sweep can reach. A tenant on Workday or Oracle only ever
	// arrives by reading it off a careers page.
	Providers []string     `json:"providers"`
	Sweepable []string     `json:"name_addressable"`
	Scorecard []scoutBoard `json:"scout_boards"`
}

func fetchWork(ctx context.Context, cfg config) (workList, error) {
	var out workList
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/api/discovery?limit=%d", cfg.api, cfg.maxTargets), nil)
	if err != nil {
		return out, err
	}
	if cfg.token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<10))
		return out, fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return out, json.NewDecoder(resp.Body).Decode(&out)
}

// recordMiss tells the server an employer was searched and not placed, so
// tomorrow's list starts with somebody else. A failure here costs one repeat
// search, not the run, so it is logged and dropped.
func recordMiss(ctx context.Context, cfg config, employer, reason string) {
	body, _ := json.Marshal(map[string]string{"employer": employer, "reason": reason})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		cfg.api+"/api/discovery/misses", bytes.NewReader(body))
	if err != nil {
		slog.Warn("recording a miss", "employer", employer, "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Warn("recording a miss", "employer", employer, "error", err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		slog.Warn("recording a miss", "employer", employer, "status", resp.Status)
	}
}

// proposeBoard hands the guess to the server, which probes it again and
// decides. Its answer is returned verbatim so the model reads its own verdict.
func proposeBoard(ctx context.Context, cfg config, provider, slug, employer, reason string,
	replaces ...string) map[string]any {
	payload := map[string]string{
		"provider": provider, "slug": slug, "employer": employer, "reason": reason,
	}
	if len(replaces) > 0 && replaces[0] != "" {
		payload["replaces"] = replaces[0]
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		cfg.api+"/api/boards", bytes.NewReader(body))
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	defer resp.Body.Close()
	var answer map[string]any
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<10)).Decode(&answer); err != nil {
		return map[string]any{"error": "the server did not answer with JSON"}
	}
	return answer
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// sweep is the list to try spellings against, falling back to everything the
// server will accept if an older deployment does not send one.
func (w workList) sweep() []string {
	if len(w.Sweepable) > 0 {
		return w.Sweepable
	}
	return w.Providers
}

// --- boards ----------------------------------------------------------------

// probeResult is what reading a board said.
type probeResult struct {
	Found     bool
	Why       string
	Postings  int
	Reachable int
	Titles    []string
	Places    []string
}

// board is a candidate: where it is, and what reading it said.
type board struct {
	Provider string
	Slug     string
	probeResult
}

func (b board) key() string { return b.Provider + ":" + b.Slug }

// probe is a variable so the run can be tested without the network.
var probe = probeBoard

// probeBoard reads a board with the same provider code the poller uses, so
// what the scout sees is what the app would store.
func probeBoard(ctx context.Context, provider, slug string) probeResult {
	fetch, known := providers.All[provider]
	if !known {
		return probeResult{Why: "no such provider: " + provider}
	}
	if strings.TrimSpace(slug) == "" {
		return probeResult{Why: "slug is required"}
	}
	probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	jobs, err := fetch(probeCtx, slug)
	if err != nil {
		return probeResult{Why: err.Error()}
	}
	r := probeResult{Found: true, Postings: len(jobs)}
	for _, j := range jobs {
		if match.Reachable(j.Location) {
			r.Reachable++
		}
		if len(r.Titles) < 6 {
			r.Titles = append(r.Titles, j.Title)
			r.Places = append(r.Places, j.Location)
		}
	}
	return r
}

// findBoards tries every spelling of the employer on every name-addressed
// system at once, and returns what answered, most reachable postings first.
func findBoards(ctx context.Context, providerNames []string, employer string) []board {
	var (
		mu   sync.Mutex
		hits []board
		wg   sync.WaitGroup
	)
	gate := make(chan struct{}, 8) // polite, and enough to finish in seconds
	for _, provider := range providerNames {
		for _, slug := range slugVariants(employer) {
			wg.Add(1)
			go func(provider, slug string) {
				defer wg.Done()
				gate <- struct{}{}
				defer func() { <-gate }()
				if r := probe(ctx, provider, slug); r.Found {
					mu.Lock()
					hits = append(hits, board{Provider: provider, Slug: slug, probeResult: r})
					mu.Unlock()
				}
			}(provider, slug)
		}
	}
	wg.Wait()
	sort.Slice(hits, func(i, j int) bool { return hits[i].Reachable > hits[j].Reachable })
	return hits
}
