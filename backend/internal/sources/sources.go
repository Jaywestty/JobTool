package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jobtool/jobtool/backend/internal/platform"
)

// RawJob is a posting as a source returned it, before canonicalisation.
type RawJob struct {
	ExternalID   string
	URL          string
	Company      string
	Title        string
	Description  string
	Location     string
	RemoteScope  string
	PostedAt     *time.Time
	Source       string
}

// Source is one job feed.
type Source interface {
	Name() string
	Fetch(ctx context.Context) ([]RawJob, error)
}

// HTTPClient is shared by all sources so timeouts and the user agent are set
// in one place.
func HTTPClient(cfg platform.Sources) *http.Client {
	timeout := time.Duration(cfg.TimeoutS) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &http.Client{Timeout: timeout}
}

// Greenhouse reads a public Greenhouse job board API. LinkedIn and Indeed are
// explicitly out of scope; this target is allowed.
type Greenhouse struct {
	Board   string
	BaseURL string
	Client  *http.Client
}

func (g Greenhouse) Name() string { return "greenhouse" }

func (g Greenhouse) baseURL() string {
	if g.BaseURL != "" {
		return strings.TrimRight(g.BaseURL, "/")
	}
	return "https://boards-api.greenhouse.io"
}

func (g Greenhouse) Fetch(ctx context.Context) ([]RawJob, error) {
	url := fmt.Sprintf("%s/v1/boards/%s/jobs?content=true", g.baseURL(), g.Board)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := g.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("greenhouse %s: status %d", g.Board, resp.StatusCode)
	}
	return ParseGreenhouse(resp.Body, g.Board)
}

// ParseGreenhouse is separated so tests can drive it with httptest bodies.
func ParseGreenhouse(r io.Reader, board string) ([]RawJob, error) {
	var payload struct {
		Jobs []struct {
			ID          int64  `json:"id"`
			Title       string `json:"title"`
			AbsoluteURL string `json:"absolute_url"`
			Content     string `json:"content"`
			UpdatedAt   string `json:"updated_at"`
			Location    struct {
				Name string `json:"name"`
			} `json:"location"`
		} `json:"jobs"`
	}
	if err := json.NewDecoder(r).Decode(&payload); err != nil {
		return nil, err
	}
	out := make([]RawJob, 0, len(payload.Jobs))
	for _, j := range payload.Jobs {
		posted := parseTime(j.UpdatedAt)
		out = append(out, RawJob{
			ExternalID:  fmt.Sprint(j.ID),
			URL:         j.AbsoluteURL,
			Company:     board,
			Title:       j.Title,
			Description: j.Content,
			Location:    j.Location.Name,
			Source:      "greenhouse:" + board,
			PostedAt:    posted,
		})
	}
	return out, nil
}

func parseTime(s string) *time.Time {
	if s == "" {
		return nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return &t
	}
	return nil
}
