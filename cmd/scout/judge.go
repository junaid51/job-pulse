package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// The one question the model is asked.
//
// Name collisions are the whole difficulty: a sweep that tries "future" for
// "Future Data" finds a fitness app, and "q1" for "Q1 Technologies" finds a
// stranger's two-posting test board. A slug resembling the name is evidence of
// nothing; what a board actually posts, against what the employer is known to
// post, is.
const judgePrompt = `You decide whether a job board belongs to a particular employer.

You are given the employer's name as a job aggregator printed it, some of that
employer's postings as the aggregator carried them, and a board found under a
similar name with a sample of what it posts.

Same company means the board is the employer's own hiring page — including a
parent, a subsidiary or a regional arm of it. Different company means a namesake:
another business that happens to share a word of the name.

How to decide:
- Short or generic slugs ("future", "q1", "reach", "base") are often namesakes.
  Do not accept a board just because its name resembles the employer's.
- Compare the work: an employer known for data engineering roles in Dubai is not
  a board hiring health coaches.
- A large employer posting in many countries is still the same company even if
  most of its postings are elsewhere.
- If the employer's known postings are missing, judge from the name and the
  board's postings alone, and lean towards "not the same" unless the board
  plainly describes that employer.
- If you are unsure, answer false. A wrong yes costs more than a wrong no.

Answer with only a JSON object, nothing else:
{"same_company": true or false, "reason": "one sentence naming what decided it"}`

// evidence is everything the judge sees about one candidate.
type evidence struct {
	Employer string
	Known    []string // "title — location" from the aggregator
	Replaces string   // the board this would replace, if the old one stopped answering
	Board    board
}

type verdict struct {
	Same   bool   `json:"same_company"`
	Reason string `json:"reason"`
}

func (e evidence) prompt() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Employer: %s\n", e.Employer)
	if e.Replaces != "" {
		fmt.Fprintf(&b, "Their previous board, %s, has stopped answering; this may be where they moved.\n", e.Replaces)
	}
	if len(e.Known) > 0 {
		b.WriteString("Known postings from this employer:\n")
		for _, k := range e.Known {
			fmt.Fprintf(&b, "- %s\n", k)
		}
	} else {
		b.WriteString("Known postings from this employer: none available\n")
	}
	fmt.Fprintf(&b, "\nCandidate board: %s:%s, %d postings\n", e.Board.Provider, e.Board.Slug, e.Board.Postings)
	for i, title := range e.Board.Titles {
		place := ""
		if i < len(e.Board.Places) {
			place = e.Board.Places[i]
		}
		fmt.Fprintf(&b, "- %s — %s\n", title, place)
	}
	return b.String()
}

// judge asks the model whether the candidate board belongs to the employer.
func judge(ctx context.Context, cfg config, e evidence) (verdict, error) {
	answer, err := complete(ctx, cfg, judgePrompt, e.prompt())
	if err != nil {
		return verdict{}, err
	}
	return parseVerdict(answer)
}

var (
	thinking   = regexp.MustCompile(`(?s)<think>.*?</think>`)
	jsonObject = regexp.MustCompile(`(?s)\{[^{}]*"same_company"[^{}]*\}`)
)

// parseVerdict reads the verdict out of whatever the model wrapped it in:
// reasoning models think out loud first, small ones add a sentence or a code
// fence. The last object naming same_company is the answer.
func parseVerdict(answer string) (verdict, error) {
	answer = thinking.ReplaceAllString(answer, "")
	objects := jsonObject.FindAllString(answer, -1)
	if len(objects) == 0 {
		return verdict{}, fmt.Errorf("no verdict in the answer: %q", truncate(strings.TrimSpace(answer), 160))
	}
	var v verdict
	if err := json.Unmarshal([]byte(objects[len(objects)-1]), &v); err != nil {
		return verdict{}, fmt.Errorf("unreadable verdict %q: %w", objects[len(objects)-1], err)
	}
	return v, nil
}

// complete sends one system and one user message and returns the reply's text.
func complete(ctx context.Context, cfg config, system, user string) (string, error) {
	request := map[string]any{
		"model": cfg.model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"temperature": 0, "stream": false,
	}
	if cfg.reasoning != "" {
		request["reasoning_effort"] = cfg.reasoning
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.modelURL, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.modelKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.modelKey)
	}
	// Generous: a reasoning model on a CPU runner thinks for a while.
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("model answered %s: %s", resp.Status, truncate(strings.TrimSpace(string(body)), 200))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("decoding the model's answer: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", errors.New("the model returned no choices")
	}
	return out.Choices[0].Message.Content, nil
}

// writeSummary puts the run, and what every board discovery has added is
// delivering, on the workflow run's page. It is the review: whether the scout
// is earning its keep is answered by postings and matches, not by its own
// account of itself.
func writeSummary(s scout, work workList) {
	path := os.Getenv("GITHUB_STEP_SUMMARY")
	if path == "" {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Scout\n\n%d added, %d refused by the server, %d candidates judged, %d rejected as namesakes.\n\n",
		s.added, s.refused, s.judged, s.rejected)
	for _, line := range s.log {
		fmt.Fprintf(&b, "- %s\n", line)
	}
	if len(work.Scorecard) > 0 {
		b.WriteString("\n### What discovered boards have delivered\n\n")
		b.WriteString("| Board | Employer | Added | Postings | In market | Matched | Status |\n|---|---|---|---|---|---|---|\n")
		for _, c := range work.Scorecard {
			status := "watching"
			if !c.Active {
				status = "retired: " + c.Rationale
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %d | %d | %d | %s |\n", c.Board, c.Employer,
				c.AddedAt.Format("2006-01-02"), c.Postings, c.InMarket, c.Matched, status)
		}
	}
	if f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644); err == nil {
		_, _ = f.WriteString(b.String())
		_ = f.Close()
	}
}
