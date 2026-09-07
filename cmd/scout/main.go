// Command scout looks for job boards JobPulse is not watching yet.
//
// It is deliberately a small thing wrapped around two ideas. The model is good
// at knowing that Majid Al Futtaim might be "majidalfuttaim" or "maf", and at
// giving up on a name after a few tries; it is not good at telling the truth
// about what it found. Asked about Air Arabia, a 3B model proposed a Greenhouse
// board it had itself just probed and found empty, reasoning that "Air Arabia
// is a major company and may have Gulf postings".
//
// So the scout may propose and may not decide. Proposing means POSTing to
// /api/boards, where the server probes the board itself and answers "watching"
// or "refused" — and that answer goes straight back into the conversation, so
// the model learns the outcome of its own guess in the same loop.
//
// It runs against any OpenAI-compatible chat endpoint, which means the free
// ones: ollama on a laptop or a CI runner, or Groq's free tier with a key.
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
	api        string
	token      string
	modelURL   string
	model      string
	modelKey   string
	maxTargets int
	maxTurns   int
}

func run() error {
	cfg := config{
		api:      env("JOBPULSE_API", "http://localhost:8091"),
		token:    os.Getenv("POLL_TOKEN"),
		modelURL: env("SCOUT_MODEL_URL", "http://localhost:11434/v1/chat/completions"),
		model:    env("SCOUT_MODEL", "qwen2.5:3b"),
		modelKey: os.Getenv("SCOUT_MODEL_KEY"),
	}
	flag.IntVar(&cfg.maxTargets, "targets", 5, "how many employers to look for")
	flag.IntVar(&cfg.maxTurns, "turns", 10, "how many model turns to allow per employer")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Minute)
	defer cancel()

	work, err := fetchWork(ctx, cfg)
	if err != nil {
		return fmt.Errorf("asking the server what is worth looking for: %w", err)
	}
	if len(work.Targets) == 0 {
		slog.Info("nothing to look for; every matched employer already has a board")
		return nil
	}
	slog.Info("scouting", "targets", len(work.Targets), "already judged", len(work.Skip),
		"model", cfg.model)

	added, refused := 0, 0
	for i, t := range work.Targets {
		if i >= cfg.maxTargets {
			break
		}
		outcome, err := hunt(ctx, cfg, work, t)
		if err != nil {
			// One employer failing is not the run failing: the next name may
			// well work, and a scout that stops at the first 429 finds nothing.
			slog.Warn("gave up on an employer", "employer", t.Employer, "error", err)
			continue
		}
		added += outcome.added
		refused += outcome.refused
	}
	slog.Info("scout finished", "boards added", added, "proposals refused", refused)
	return nil
}

// --- what the server knows ------------------------------------------------

type target struct {
	Employer string `json:"employer"`
	Matches  int    `json:"matches"`
}

type judged struct {
	Provider string `json:"provider"`
	Slug     string `json:"slug"`
	Verdict  string `json:"verdict"`
}

type workList struct {
	Targets []target `json:"targets"`
	Skip    []judged `json:"do_not_try"`
	// Providers is everything that may be proposed; Sweepable is the narrower
	// set a spelling sweep can reach. A tenant on Workday or Oracle only ever
	// arrives by reading it off a careers page.
	Providers []string `json:"providers"`
	Sweepable []string `json:"name_addressable"`
}

// sweep is the list to try spellings against, falling back to everything the
// server will accept if an older deployment does not send one.
func (w workList) sweep() []string {
	if len(w.Sweepable) > 0 {
		return w.Sweepable
	}
	return w.Providers
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

// --- the tools -------------------------------------------------------------

type outcome struct{ added, refused int }

// probe is a variable so the loop can be driven in tests without the network.
var probe = probeBoard

// probeBoard reads a board with the same provider code the poller uses, so
// what the scout sees is what the app would store.
func probeBoard(ctx context.Context, provider, slug string) map[string]any {
	fetch, known := providers.All[provider]
	if !known {
		return map[string]any{"error": "no such provider: " + provider}
	}
	if strings.TrimSpace(slug) == "" {
		return map[string]any{"error": "slug is required"}
	}
	probe, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	jobs, err := fetch(probe, slug)
	if err != nil {
		return map[string]any{"found": false, "why": err.Error()}
	}
	reachable, titles, places := 0, []string{}, []string{}
	for _, j := range jobs {
		if match.Reachable(j.Location) {
			reachable++
		}
		if len(titles) < 5 {
			titles = append(titles, j.Title)
			places = append(places, j.Location)
		}
	}
	return map[string]any{
		"found": true, "postings": len(jobs), "reachable_postings": reachable,
		"sample_titles": titles, "sample_locations": places,
	}
}

// findBoards sweeps every sensible spelling of an employer's name across every
// system it could be on, at once. This is the work the models would not do:
// both of them probed one spelling six times and stopped.
func findBoards(ctx context.Context, providerNames []string, employer string) map[string]any {
	variants := slugVariants(employer)
	type hit struct {
		Provider  string   `json:"provider"`
		Slug      string   `json:"slug"`
		Postings  int      `json:"postings"`
		Reachable int      `json:"reachable_postings"`
		Titles    []string `json:"sample_titles"`
		Locations []string `json:"sample_locations"`
	}

	var (
		mu   sync.Mutex
		hits []hit
		wg   sync.WaitGroup
	)
	gate := make(chan struct{}, 8) // polite, and enough to finish in seconds
	for _, provider := range providerNames {
		for _, slug := range variants {
			wg.Add(1)
			go func(provider, slug string) {
				defer wg.Done()
				gate <- struct{}{}
				defer func() { <-gate }()
				answer := probe(ctx, provider, slug)
				if found, _ := answer["found"].(bool); !found {
					return
				}
				n, _ := answer["reachable_postings"].(int)
				total, _ := answer["postings"].(int)
				titles, _ := answer["sample_titles"].([]string)
				places, _ := answer["sample_locations"].([]string)
				mu.Lock()
				hits = append(hits, hit{provider, slug, total, n, titles, places})
				mu.Unlock()
			}(provider, slug)
		}
	}
	wg.Wait()
	sort.Slice(hits, func(i, j int) bool { return hits[i].Reachable > hits[j].Reachable })
	return map[string]any{
		"spellings_tried": variants,
		"systems_tried":   providerNames,
		"hits":            hits,
	}
}

// proposeBoard hands the guess to the server, which probes it again and
// decides. Its answer is returned verbatim so the model reads its own verdict.
func proposeBoard(ctx context.Context, cfg config, provider, slug, employer, reason string) map[string]any {
	body, _ := json.Marshal(map[string]string{
		"provider": provider, "slug": slug, "employer": employer, "reason": reason,
	})
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

func toolSpecs(providerNames []string) []map[string]any {
	sort.Strings(providerNames)
	return []map[string]any{
		{"type": "function", "function": map[string]any{
			"name":        "find_boards",
			"description": "Search every hiring system for an employer, trying the usual spellings of their name. Start here. If it finds nothing, try again with a shorter or more common form of the name — 'Halian | Managed Services, Recruitment Agency' is really just 'Halian'.",
			"parameters": map[string]any{"type": "object", "properties": map[string]any{
				"employer": map[string]any{"type": "string", "description": "the employer's name, as plainly as you can write it"},
			}, "required": []string{"employer"}},
		}},
		{"type": "function", "function": map[string]any{
			"name":        "find_hiring_system",
			"description": "Open every plausible careers page for an employer at once and report which hiring systems they name. Use this when find_boards comes up empty: large employers run systems addressed by a tenant address rather than their name, and their careers page gives it away.",
			"parameters": map[string]any{"type": "object", "properties": map[string]any{
				"employer": map[string]any{"type": "string"},
			}, "required": []string{"employer"}},
		}},
		{"type": "function", "function": map[string]any{
			"name":        "read_careers_page",
			"description": "Open a company's careers page and report which hiring system it uses. Use this when find_boards comes up empty: big employers run systems addressed by a tenant address rather than their name, and the page gives it away. Try careers.<company>.com, www.<company>.com/careers, or a link this tool suggested.",
			"parameters": map[string]any{"type": "object", "properties": map[string]any{
				"url": map[string]any{"type": "string", "description": "the page to open, with https://"},
			}, "required": []string{"url"}},
		}},
		{"type": "function", "function": map[string]any{
			"name":        "probe_board",
			"description": "Read an employer's job board on one applicant tracking system. Says how many postings it has and how many are somewhere this job hunt can reach.",
			"parameters": map[string]any{"type": "object", "properties": map[string]any{
				"provider": map[string]any{"type": "string", "enum": providerNames},
				"slug":     map[string]any{"type": "string", "description": "the employer's id on that system, usually its name lowercased without spaces"},
			}, "required": []string{"provider", "slug"}},
		}},
		{"type": "function", "function": map[string]any{
			"name":        "propose_board",
			"description": "Offer a board for JobPulse to watch. The server checks it again and may refuse; its answer tells you what happened. Only worth calling after probe_board showed reachable postings.",
			"parameters": map[string]any{"type": "object", "properties": map[string]any{
				"provider": map[string]any{"type": "string", "enum": providerNames},
				"slug":     map[string]any{"type": "string"},
				"employer": map[string]any{"type": "string"},
				"reason":   map[string]any{"type": "string", "description": "what you saw that justifies watching it"},
			}, "required": []string{"provider", "slug", "employer", "reason"}},
		}},
	}
}

const systemPrompt = `You find job boards for a job hunt in the Gulf and India.

Given an employer, work out whether they publish jobs on a hiring system this
app can read, and propose it if they do.

How to work:
- Call find_boards first with the employer's name. It searches the systems that
  are addressed by a company's name and tries the usual spellings, so one call
  does most of the work.
- If that finds nothing, call find_hiring_system with the employer's name. It
  opens their careers pages and reports the hiring system each one names. Large
  employers run systems addressed by a tenant address rather than their name, so
  this is the only way to reach them: every spelling of "Etihad" missed, while
  their careers page said "EtihadAirways5" — ninety-eight postings, half of them
  in Abu Dhabi.
- Whatever it reports, probe it before proposing it.
- read_careers_page opens one specific page, for following a link another tool
  suggested.
- If it finds nothing, the name may be the problem rather than the employer.
  Company names in job postings carry taglines and legal wrappers:
  "Halian | Managed Services, Recruitment Agency" is Halian, and
  "Bespin Global MEA, an e& enterprise company" is Bespin Global. Try again with
  the plain name. Two attempts is usually enough.
- A board can exist and still belong to someone else entirely. Read the sample
  titles and locations: if they do not look like the employer you were asked
  about, it is not them, however many postings there are.
- Postings nobody on this hunt could take are worth nothing, whatever the count.
- Propose at most one board, and only one that was actually found.
- If nothing turns up, say so plainly and stop. Plenty of employers have no
  readable board, and a guess is worse than nothing.`

// --- the loop --------------------------------------------------------------

type chatMessage struct {
	Role string `json:"role"`
	// Never omitempty: an assistant turn that is only tool calls has empty
	// content, and a server that receives the field missing rather than empty
	// answers "invalid message content type: <nil>" on the next request.
	Content    string     `json:"content"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func hunt(ctx context.Context, cfg config, work workList, t target) (outcome, error) {
	var result outcome
	skip := map[string]bool{}
	for _, j := range work.Skip {
		skip[j.Provider+":"+j.Slug] = true
	}

	// Every board this employer was actually found on, so a proposal can be
	// bound to evidence the loop gathered rather than to what the model
	// remembers. A 3B model found smartrecruiters:namshi, then called
	// propose_board twice without the slug and signed off saying it had
	// proposed the board. Neither the claim nor the omission survives this.
	probed := map[string]map[string]any{}
	proposedAlready := map[string]bool{}
	nudged := false

	messages := []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: fmt.Sprintf(
			"Find the job board for: %s\n(%d of this reader's matches come from this employer, but only through an aggregator, which means they arrive hours late.)",
			t.Employer, t.Matches)},
	}
	tools := toolSpecs(work.Providers)

	for turn := 0; turn < cfg.maxTurns; turn++ {
		reply, err := ask(ctx, cfg, messages, tools)
		if err != nil {
			return result, err
		}
		messages = append(messages, reply)
		if len(reply.ToolCalls) == 0 {
			// A small model will describe the action instead of taking it: it
			// found ashby:leantech, said "the job board has been proposed for
			// monitoring", and called nothing. One nudge, because the decision
			// is still the model's — it may have looked at the postings and
			// concluded they belong to a different company, and that judgement
			// is the part worth keeping.
			if only, ok := solePending(probed, proposedAlready); ok && !nudged {
				nudged = true
				messages = append(messages, chatMessage{Role: "user", Content: fmt.Sprintf(
					"You did not call propose_board. If %s:%s is really %s, call propose_board for it now. If it is not them, say which part of what you saw says so.",
					only.provider, only.slug, t.Employer)})
				continue
			}
			slog.Info("done with employer", "employer", t.Employer,
				"said", firstLine(reply.Content))
			return result, nil
		}
		for _, call := range reply.ToolCalls {
			args := map[string]string{}
			_ = json.Unmarshal([]byte(call.Function.Arguments), &args)
			var answer map[string]any

			switch call.Function.Name {
			case "find_boards":
				name := args["employer"]
				if strings.TrimSpace(name) == "" {
					name = t.Employer
				}
				answer = findBoards(ctx, work.sweep(), name)
				// Record the hits so a proposal can be bound to one of them.
				encoded, _ := json.Marshal(answer["hits"])
				var hits []map[string]any
				_ = json.Unmarshal(encoded, &hits)
				for _, h := range hits {
					if n, _ := h["reachable_postings"].(float64); n > 0 {
						provider, _ := h["provider"].(string)
						slug, _ := h["slug"].(string)
						probed[provider+":"+slug] = h
					}
				}
			case "find_hiring_system":
				name := args["employer"]
				if strings.TrimSpace(name) == "" {
					name = t.Employer
				}
				answer = findHiringSystem(ctx, name)
			case "read_careers_page":
				answer = readCareersPage(ctx, args["url"])
			case "probe_board":
				if skip[args["provider"]+":"+args["slug"]] {
					// Already judged once. Saying so is cheaper than probing,
					// and it is what stops a tireless scout re-probing the same
					// dead board every week.
					answer = map[string]any{"skipped": "this board has already been judged"}
				} else {
					answer = probe(ctx, args["provider"], args["slug"])
					if found, _ := answer["found"].(bool); found {
						if n, _ := answer["reachable_postings"].(int); n > 0 {
							probed[args["provider"]+":"+args["slug"]] = answer
						}
					}
				}
			case "propose_board":
				provider, slug := args["provider"], args["slug"]
				// A dropped argument is the commonest small-model failure, and
				// an unbound proposal is the most dangerous. If exactly one
				// board was found for this employer, that is the one being
				// proposed; anything else has to name a board actually probed.
				if only, ok := soleProbe(probed, provider, slug); ok {
					provider, slug = only.provider, only.slug
				}
				if _, seen := probed[provider+":"+slug]; !seen {
					answer = map[string]any{"error": "propose only a board you probed and " +
						"found reachable postings on; you have found " + strings.Join(names(probed), ", ")}
					break
				}
				proposedAlready[provider+":"+slug] = true
				answer = proposeBoard(ctx, cfg, provider, slug,
					args["employer"], args["reason"])
				switch answer["status"] {
				case "watching":
					result.added++
				case "refused":
					result.refused++
				}
				skip[args["provider"]+":"+args["slug"]] = true
			default:
				answer = map[string]any{"error": "no such tool: " + call.Function.Name}
			}

			encoded, _ := json.Marshal(answer)
			slog.Info("tool", "employer", t.Employer, "call", call.Function.Name,
				"args", call.Function.Arguments, "answer", truncate(string(encoded), 160))
			messages = append(messages, chatMessage{
				Role: "tool", ToolCallID: call.ID, Content: string(encoded),
			})
		}
	}
	slog.Warn("ran out of turns", "employer", t.Employer)
	return result, nil
}

func ask(ctx context.Context, cfg config, messages []chatMessage, tools []map[string]any) (chatMessage, error) {
	payload, err := json.Marshal(map[string]any{
		"model": cfg.model, "messages": messages, "tools": tools,
		"temperature": 0, "stream": false,
	})
	if err != nil {
		return chatMessage{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.modelURL,
		bytes.NewReader(payload))
	if err != nil {
		return chatMessage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.modelKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.modelKey)
	}
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return chatMessage{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return chatMessage{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return chatMessage{}, fmt.Errorf("model answered %s: %s",
			resp.Status, truncate(strings.TrimSpace(string(body)), 200))
	}
	var out struct {
		Choices []struct {
			Message chatMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return chatMessage{}, fmt.Errorf("decoding the model's answer: %w", err)
	}
	if len(out.Choices) == 0 {
		return chatMessage{}, fmt.Errorf("the model returned no choices")
	}
	return out.Choices[0].Message, nil
}

type pair struct{ provider, slug string }

// solePending is the one board that was found and not yet offered, if there is
// exactly one. Anything else is a judgement call and stays with the model.
func solePending(probed map[string]map[string]any, proposed map[string]bool) (pair, bool) {
	var only pair
	count := 0
	for key := range probed {
		if proposed[key] {
			continue
		}
		p, s, _ := strings.Cut(key, ":")
		only = pair{provider: p, slug: s}
		count++
	}
	return only, count == 1
}

// soleProbe answers the case where the model left the board underspecified and
// there is only one it could mean.
func soleProbe(probed map[string]map[string]any, provider, slug string) (pair, bool) {
	if provider != "" && slug != "" {
		return pair{}, false
	}
	if len(probed) != 1 {
		return pair{}, false
	}
	for key := range probed {
		p, s, _ := strings.Cut(key, ":")
		return pair{provider: p, slug: s}, true
	}
	return pair{}, false
}

func names(probed map[string]map[string]any) []string {
	out := make([]string, 0, len(probed))
	for key := range probed {
		out = append(out, key)
	}
	if len(out) == 0 {
		return []string{"nothing yet"}
	}
	sort.Strings(out)
	return out
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

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return truncate(s, 120)
}
