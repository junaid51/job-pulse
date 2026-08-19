package providers

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// greenhouseBoard is the shape of
// https://boards-api.greenhouse.io/v1/boards/{slug}/jobs
//
// content=true is deliberately not requested: it inlines the full job
// description and multiplies the payload by roughly twelve (Stripe's board goes
// from 360 KB to 4.4 MB), and JobPulse does not store descriptions.
type greenhouseBoard struct {
	Jobs []struct {
		ID             int64  `json:"id"`
		Title          string `json:"title"`
		AbsoluteURL    string `json:"absolute_url"`
		CompanyName    string `json:"company_name"`
		FirstPublished string `json:"first_published"`
		UpdatedAt      string `json:"updated_at"`
		Location       struct {
			Name string `json:"name"`
		} `json:"location"`
	} `json:"jobs"`
}

func fetchGreenhouse(ctx context.Context, slug string) ([]Job, error) {
	var board greenhouseBoard
	if err := getJSON(ctx, "https://boards-api.greenhouse.io/v1/boards/"+slug+"/jobs", &board); err != nil {
		return nil, err
	}
	return board.jobs(), nil
}

func (b greenhouseBoard) jobs() []Job {
	jobs := make([]Job, 0, len(b.Jobs))
	for _, j := range b.Jobs {
		posted := parseTime(time.RFC3339, j.FirstPublished)
		if posted.IsZero() {
			posted = parseTime(time.RFC3339, j.UpdatedAt)
		}
		jobs = append(jobs, Job{
			ExternalID: strconv.FormatInt(j.ID, 10),
			Company:    j.CompanyName,
			Title:      strings.TrimSpace(j.Title),
			Location:   j.Location.Name,
			// Greenhouse has no remote flag; the board only says so in the text.
			Remote:   looksRemote(j.Location.Name),
			URL:      j.AbsoluteURL,
			PostedAt: posted,
		})
	}
	return jobs
}
