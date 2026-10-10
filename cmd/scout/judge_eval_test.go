//go:build judgeeval

package main

// The judge's exam. Not part of the normal test run — it needs a model:
//
//	SCOUT_MODEL=qwen2.5:7b go test -tags judgeeval -run TestJudgeExam -v ./cmd/scout/
//
// Every case is real or built from real boards, labelled by hand from what the
// board posts against what the employer is known to post. A wrong yes is the
// costly mistake: it puts a namesake's postings into the feed.

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

type examCase struct {
	Employer string   `json:"employer"`
	Known    []string `json:"known"`
	Provider string   `json:"provider"`
	Slug     string   `json:"slug"`
	Postings int      `json:"postings"`
	Titles   []string `json:"titles"`
	Places   []string `json:"places"`
	Same     bool     `json:"same"`
	Note     string   `json:"note"`
}

func TestJudgeExam(t *testing.T) {
	raw, err := os.ReadFile("testdata/judge_cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []examCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	cfg := config{
		modelURL:  env("SCOUT_MODEL_URL", "http://localhost:11434/v1/chat/completions"),
		model:     env("SCOUT_MODEL", "qwen2.5:7b"),
		modelKey:  os.Getenv("SCOUT_MODEL_KEY"),
		reasoning: os.Getenv("SCOUT_MODEL_REASONING"),
	}
	var right, wrongYes, wrongNo, failed int
	var total time.Duration
	for _, c := range cases {
		start := time.Now()
		v, err := judge(t.Context(), cfg, evidence{Employer: c.Employer, Known: c.Known, Board: board{
			Provider: c.Provider, Slug: c.Slug,
			probeResult: probeResult{Found: true, Postings: c.Postings, Titles: c.Titles, Places: c.Places},
		}})
		took := time.Since(start)
		total += took
		mark := "ok  "
		switch {
		case err != nil:
			failed++
			mark = "FAIL"
		case v.Same == c.Same:
			right++
		case v.Same:
			wrongYes++
			mark = "YES!"
		default:
			wrongNo++
			mark = "no? "
		}
		t.Logf("%s %5.1fs  %-28s -> %-32s want %-5v got %-5v  %s", mark, took.Seconds(), c.Employer,
			c.Provider+":"+c.Slug, c.Same, v.Same, truncate(v.Reason, 90))
		if err != nil {
			t.Logf("      error: %v", err)
		}
	}
	t.Logf("MODEL %s reasoning=%q: %d/%d right, %d wrong yes, %d wrong no, %d unanswered, %.1fs per case",
		cfg.model, cfg.reasoning, right, len(cases), wrongYes, wrongNo, failed, total.Seconds()/float64(len(cases)))
}
