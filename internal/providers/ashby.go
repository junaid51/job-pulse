package providers

import (
	"context"
	"strings"
	"time"
)

// ashbyBoard is the shape of
// https://api.ashbyhq.com/posting-api/job-board/{slug}
//
// Descriptions are always inlined and cannot be excluded, so these payloads are
// large (OpenAI's board is ~12 MB). The struct simply omits them, which keeps
// the decoded result small.
type ashbyBoard struct {
	Jobs []struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Location    string `json:"location"`
		IsRemote    *bool  `json:"isRemote"` // frequently null
		IsListed    *bool  `json:"isListed"`
		JobURL      string `json:"jobUrl"`
		PublishedAt string `json:"publishedAt"`
	} `json:"jobs"`
}

func fetchAshby(ctx context.Context, slug string) ([]Job, error) {
	var board ashbyBoard
	if err := getJSON(ctx, "https://api.ashbyhq.com/posting-api/job-board/"+slug, &board); err != nil {
		return nil, err
	}
	return board.jobs(), nil
}

func (b ashbyBoard) jobs() []Job {
	jobs := make([]Job, 0, len(b.Jobs))
	for _, j := range b.Jobs {
		if j.IsListed != nil && !*j.IsListed {
			continue
		}
		remote := j.IsRemote != nil && *j.IsRemote
		jobs = append(jobs, Job{
			ExternalID: j.ID,
			// Ashby does not report the company name; the poller fills it in.
			Title:    strings.TrimSpace(j.Title),
			Location: j.Location,
			Remote:   remote || looksRemote(j.Location),
			URL:      j.JobURL,
			PostedAt: parseTime(time.RFC3339, j.PublishedAt),
		})
	}
	return jobs
}
