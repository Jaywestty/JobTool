package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	defaultGreenhouseBase = "https://boards-api.greenhouse.io"
	defaultLeverBase      = "https://api.lever.co"
	defaultAshbyBase      = "https://api.ashbyhq.com"
)

// ---- Greenhouse -----------------------------------------------------------

type greenhouseSource struct {
	slug     string
	client   *Client
	base     string
	interval int
}

func (s *greenhouseSource) Name() string         { return prefixGreenhouse + s.slug }
func (s *greenhouseSource) Interval() int        { return s.interval }
func (s *greenhouseSource) RequiresRobots() bool { return false }

func (s *greenhouseSource) Validate(ctx context.Context) error {
	_, err := s.client.Get(ctx, fmt.Sprintf("%s/v1/boards/%s", s.base, s.slug))
	return err
}

func (s *greenhouseSource) Fetch(ctx context.Context) ([]RawJob, error) {
	u := fmt.Sprintf("%s/v1/boards/%s/jobs?content=true", s.base, s.slug)
	resp, err := s.client.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	return parseGreenhouse(resp.Body, s.slug)
}

func parseGreenhouse(body []byte, slug string) ([]RawJob, error) {
	var payload struct {
		Jobs []struct {
			ID          json.Number `json:"id"`
			Title       string      `json:"title"`
			AbsoluteURL string      `json:"absolute_url"`
			Content     string      `json:"content"`
			UpdatedAt   string      `json:"updated_at"`
			Location    struct {
				Name string `json:"name"`
			} `json:"location"`
			Offices []struct {
				Name string `json:"name"`
			} `json:"offices"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("greenhouse: decode: %w", err)
	}
	out := make([]RawJob, 0, len(payload.Jobs))
	for _, j := range payload.Jobs {
		if j.AbsoluteURL == "" {
			continue
		}
		loc := j.Location.Name
		if loc == "" && len(j.Offices) > 0 {
			loc = j.Offices[0].Name
		}
		remote := strings.Contains(strings.ToLower(loc), "remote")
		out = append(out, RawJob{
			ExternalID:  j.ID.String(),
			Title:       j.Title,
			Company:     slug,
			Description: StripHTML(j.Content),
			Location:    loc,
			URL:         j.AbsoluteURL,
			PostedAt:    j.UpdatedAt,
			IsRemote:    &remote,
		})
	}
	return out, nil
}

// ---- Lever ----------------------------------------------------------------

type leverSource struct {
	slug     string
	client   *Client
	base     string
	interval int
}

func (s *leverSource) Name() string         { return prefixLever + s.slug }
func (s *leverSource) Interval() int        { return s.interval }
func (s *leverSource) RequiresRobots() bool { return false }

func (s *leverSource) Validate(ctx context.Context) error {
	_, err := s.client.Get(ctx, fmt.Sprintf("%s/v0/postings/%s", s.base, s.slug))
	return err
}

func (s *leverSource) Fetch(ctx context.Context) ([]RawJob, error) {
	resp, err := s.client.Get(ctx, fmt.Sprintf("%s/v0/postings/%s", s.base, s.slug))
	if err != nil {
		return nil, err
	}
	return parseLever(resp.Body, s.slug)
}

func parseLever(body []byte, slug string) ([]RawJob, error) {
	var items []struct {
		Text        string          `json:"text"`
		HostedURL   string          `json:"hostedUrl"`
		ApplyURL    string          `json:"applyUrl"`
		Description string          `json:"description"`
		CreatedAt   json.RawMessage `json:"createdAt"`
		DatePosted  json.RawMessage `json:"datePosted"`
		Workplace   string          `json:"workplaceType"`
		Categories  struct {
			Location      string `json:"location"`
			WorkplaceType string `json:"workplaceType"`
			Commitment    string `json:"commitment"`
		} `json:"categories"`
		Lists []struct {
			Content string `json:"content"`
		} `json:"lists"`
	}
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, fmt.Errorf("lever: decode: %w", err)
	}
	out := make([]RawJob, 0, len(items))
	for _, it := range items {
		u := it.HostedURL
		if u == "" {
			u = it.ApplyURL
		}
		if u == "" {
			continue
		}
		loc := it.Categories.Location
		workplace := firstNonEmpty(it.Workplace, it.Categories.WorkplaceType)
		remote := strings.Contains(strings.ToLower(loc+" "+workplace), "remote")
		desc := StripHTML(it.Description)
		if desc == "" && len(it.Lists) > 0 {
			var parts []string
			for _, l := range it.Lists {
				parts = append(parts, StripHTML(l.Content))
			}
			desc = StripHTML(strings.Join(parts, " "))
		}
		posted := jsonRawString(it.CreatedAt)
		if posted == "" {
			posted = jsonRawString(it.DatePosted)
		}
		out = append(out, RawJob{
			Title:       it.Text,
			Company:     slug,
			Description: desc,
			Location:    loc,
			URL:         u,
			PostedAt:    posted,
			IsRemote:    &remote,
		})
	}
	return out, nil
}

// jsonRawString renders a JSON number or string as text; milliseconds and ISO
// both survive.
func jsonRawString(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return ""
	}
	return strings.Trim(s, `"`)
}

// ---- Ashby ----------------------------------------------------------------

type ashbySource struct {
	slug     string
	client   *Client
	base     string
	interval int
}

func (s *ashbySource) Name() string         { return prefixAshby + s.slug }
func (s *ashbySource) Interval() int        { return s.interval }
func (s *ashbySource) RequiresRobots() bool { return false }

func (s *ashbySource) Validate(ctx context.Context) error {
	_, err := s.client.Get(ctx, fmt.Sprintf("%s/posting-api/job-board/%s", s.base, s.slug))
	return err
}

func (s *ashbySource) Fetch(ctx context.Context) ([]RawJob, error) {
	u := fmt.Sprintf("%s/posting-api/job-board/%s?includeCompensation=true", s.base, s.slug)
	resp, err := s.client.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	return parseAshby(resp.Body, s.slug)
}

func parseAshby(body []byte, slug string) ([]RawJob, error) {
	var payload struct {
		Jobs []struct {
			Title              string            `json:"title"`
			JobURL             string            `json:"jobUrl"`
			DescriptionHTML    string            `json:"descriptionHtml"`
			DescriptionPlain   string            `json:"descriptionPlain"`
			Location           string            `json:"location"`
			IsRemote           bool              `json:"isRemote"`
			WorkplaceType      string            `json:"workplaceType"`
			PublishedAt        string            `json:"publishedAt"`
			SecondaryLocations []json.RawMessage `json:"secondaryLocations"`
			Compensation       struct {
				Tiers []struct {
					Min float64 `json:"min"`
					Max float64 `json:"max"`
				} `json:"compensationTiers"`
				Summary    string `json:"compensationTierSummary"`
				Scrapeable string `json:"scrapeableCompensationSalarySummary"`
			} `json:"compensation"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("ashby: decode: %w", err)
	}
	out := make([]RawJob, 0, len(payload.Jobs))
	for _, j := range payload.Jobs {
		if j.JobURL == "" || j.Title == "" {
			continue
		}
		desc := StripHTML(j.DescriptionHTML)
		if desc == "" {
			desc = StripHTML(j.DescriptionPlain)
		}
		loc := j.Location
		for _, sec := range j.SecondaryLocations {
			if name := ashbySecondaryName(sec); name != "" {
				loc = loc + ", " + name
			}
		}
		remote := j.IsRemote || strings.Contains(strings.ToLower(j.WorkplaceType+" "+loc), "remote")
		if loc == "" && remote {
			loc = "Remote"
		}
		var smin, smax *float64
		for _, tier := range j.Compensation.Tiers {
			if tier.Min > 0 || tier.Max > 0 {
				lo, hi := tier.Min, tier.Max
				smin, smax = &lo, &hi
				break
			}
		}
		row := RawJob{
			Title:       j.Title,
			Company:     slug,
			Description: truncate(desc, 12000),
			Location:    loc,
			URL:         j.JobURL,
			PostedAt:    j.PublishedAt,
			IsRemote:    &remote,
			SalaryMin:   smin,
			SalaryMax:   smax,
		}
		if smin == nil {
			if mn, mx, cur := ParseSalary(firstNonEmpty(j.Compensation.Scrapeable, j.Compensation.Summary), nil, nil, nil); mn != nil {
				fmin, fmax := float64(*mn), float64(*mx)
				row.SalaryMin, row.SalaryMax, row.SalaryCurrency = &fmin, &fmax, cur
			}
		}
		out = append(out, row)
	}
	return out, nil
}

func ashbySecondaryName(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if strings.HasPrefix(s, `"`) {
		var v string
		if json.Unmarshal(raw, &v) == nil {
			return v
		}
	}
	var obj struct {
		Location string `json:"location"`
		Name     string `json:"name"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		return firstNonEmpty(obj.Location, obj.Name)
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
