package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

const (
	defaultRemoteOKBase      = "https://remoteok.com"
	defaultRemotiveBase      = "https://remotive.com"
	defaultHimalayasBase     = "https://himalayas.app"
	defaultJobicyBase        = "https://jobicy.com"
	defaultArbeitnowBase     = "https://www.arbeitnow.com"
	defaultWorkingNomadsBase = "https://www.workingnomads.com"
)

// ---- RemoteOK -------------------------------------------------------------

type remoteOKSource struct {
	client   *Client
	base     string
	interval int
}

func (s *remoteOKSource) Name() string         { return "remoteok" }
func (s *remoteOKSource) Interval() int        { return s.interval }
func (s *remoteOKSource) RequiresRobots() bool { return false }

func (s *remoteOKSource) Fetch(ctx context.Context) ([]RawJob, error) {
	resp, err := s.client.Get(ctx, s.base+"/api")
	if err != nil {
		return nil, err
	}
	return parseRemoteOK(resp.Body)
}

func parseRemoteOK(body []byte) ([]RawJob, error) {
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, fmt.Errorf("remoteok: decode: %w", err)
	}
	var out []RawJob
	for _, item := range items {
		if _, ok := item["position"]; !ok {
			continue
		}
		u := jsonString(item["apply_url"])
		if u == "" {
			u = jsonString(item["url"])
		}
		if u == "" {
			continue
		}
		loc := firstNonEmpty(jsonString(item["location"]), "Remote")
		remote := true
		row := RawJob{
			Title:       jsonString(item["position"]),
			Company:     jsonString(item["company"]),
			Description: StripHTML(jsonString(item["description"])),
			Location:    loc,
			URL:         u,
			IsRemote:    &remote,
		}
		if ep := jsonNumber(item["epoch"]); ep != nil {
			row.PostedEpoch = ep
		} else {
			row.PostedAt = jsonString(item["date"])
		}
		row.SalaryMin = jsonNumber(item["salary_min"])
		row.SalaryMax = jsonNumber(item["salary_max"])
		out = append(out, row)
	}
	return out, nil
}

// ---- Remotive -------------------------------------------------------------

type remotiveSource struct {
	client   *Client
	base     string
	searches []string
	interval int
}

func (s *remotiveSource) Name() string         { return "remotive" }
func (s *remotiveSource) Interval() int        { return s.interval }
func (s *remotiveSource) RequiresRobots() bool { return false }

func (s *remotiveSource) Fetch(ctx context.Context) ([]RawJob, error) {
	if len(s.searches) == 0 {
		return nil, nil
	}
	var out []RawJob
	for _, search := range s.searches {
		u := s.base + "/api/remote-jobs?search=" + url.QueryEscape(search)
		resp, err := s.client.Get(ctx, u)
		if err != nil {
			return out, err
		}
		jobs, err := parseRemotive(resp.Body)
		if err != nil {
			return out, err
		}
		out = append(out, jobs...)
	}
	return out, nil
}

func parseRemotive(body []byte) ([]RawJob, error) {
	var payload struct {
		Jobs []struct {
			Title       string `json:"title"`
			CompanyName string `json:"company_name"`
			Description string `json:"description"`
			URL         string `json:"url"`
			Location    string `json:"candidate_required_location"`
			PublishedAt string `json:"publication_date"`
			Salary      string `json:"salary"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("remotive: decode: %w", err)
	}
	var out []RawJob
	for _, j := range payload.Jobs {
		if j.URL == "" {
			continue
		}
		remote := true
		out = append(out, RawJob{
			Title:       j.Title,
			Company:     j.CompanyName,
			Description: StripHTML(j.Description),
			Location:    firstNonEmpty(j.Location, "Remote"),
			URL:         j.URL,
			PostedAt:    j.PublishedAt,
			IsRemote:    &remote,
		})
	}
	return out, nil
}

// ---- Himalayas ------------------------------------------------------------

type himalayasSource struct {
	client   *Client
	base     string
	maxPages int
	interval int
}

func (s *himalayasSource) Name() string         { return "himalayas" }
func (s *himalayasSource) Interval() int        { return s.interval }
func (s *himalayasSource) RequiresRobots() bool { return false }

func (s *himalayasSource) Fetch(ctx context.Context) ([]RawJob, error) {
	var out []RawJob
	seen := map[string]bool{}
	cursor := ""
	for page := 0; page < s.maxPages; page++ {
		u := s.base + "/jobs/api"
		if cursor != "" {
			u += "?cursor=" + url.QueryEscape(cursor)
		}
		resp, err := s.client.Get(ctx, u)
		if err != nil {
			return out, err
		}
		jobs, next, err := parseHimalayasPage(resp.Body)
		if err != nil {
			return out, err
		}
		out = append(out, jobs...)
		if next == "" {
			break
		}
		if seen[next] {
			return out, fmt.Errorf("himalayas: cursor repeated (%s)", next)
		}
		seen[next] = true
		cursor = next
	}
	return out, nil
}

func parseHimalayasPage(body []byte) ([]RawJob, string, error) {
	var payload struct {
		Jobs []struct {
			Title                string          `json:"title"`
			CompanyName          string          `json:"companyName"`
			Description          string          `json:"description"`
			ApplicationLink      string          `json:"applicationLink"`
			GUID                 string          `json:"guid"`
			PubDate              json.RawMessage `json:"pubDate"`
			MinSalary            *float64        `json:"minSalary"`
			MaxSalary            *float64        `json:"maxSalary"`
			LocationRestrictions []string        `json:"locationRestrictions"`
			Seniority            []string        `json:"seniority"`
		} `json:"jobs"`
		NextCursor string `json:"nextCursor"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, "", fmt.Errorf("himalayas: decode: %w", err)
	}
	var out []RawJob
	for _, j := range payload.Jobs {
		u := firstNonEmpty(j.ApplicationLink, j.GUID)
		if u == "" {
			continue
		}
		remote := true
		row := RawJob{
			Title:           j.Title,
			Company:         j.CompanyName,
			Description:     StripHTML(j.Description),
			Location:        firstNonEmpty(strings.Join(j.LocationRestrictions, ", "), "Remote"),
			URL:             u,
			IsRemote:        &remote,
			SalaryMin:       j.MinSalary,
			SalaryMax:       j.MaxSalary,
			NativeSeniority: strings.Join(j.Seniority, ", "),
		}
		if len(j.PubDate) > 0 {
			if ep := jsonNumber(j.PubDate); ep != nil {
				row.PostedEpoch = ep
			} else {
				row.PostedAt = jsonString(j.PubDate)
			}
		}
		out = append(out, row)
	}
	return out, payload.NextCursor, nil
}

// ---- Jobicy ---------------------------------------------------------------

type jobicySource struct {
	client   *Client
	base     string
	count    int
	interval int
}

func (s *jobicySource) Name() string         { return "jobicy" }
func (s *jobicySource) Interval() int        { return s.interval }
func (s *jobicySource) RequiresRobots() bool { return false }

func (s *jobicySource) Fetch(ctx context.Context) ([]RawJob, error) {
	u := fmt.Sprintf("%s/api/v2/remote-jobs?count=%d&industry=engineering", s.base, s.count)
	resp, err := s.client.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	return parseJobicy(resp.Body)
}

func parseJobicy(body []byte) ([]RawJob, error) {
	var payload struct {
		Success bool `json:"success"`
		Jobs    []struct {
			JobTitle       string   `json:"jobTitle"`
			CompanyName    string   `json:"companyName"`
			Description    string   `json:"jobDescription"`
			URL            string   `json:"url"`
			Geo            string   `json:"jobGeo"`
			PubDate        string   `json:"pubDate"`
			SalaryMin      *float64 `json:"salaryMin"`
			SalaryMax      *float64 `json:"salaryMax"`
			SalaryCurrency string   `json:"salaryCurrency"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("jobicy: decode: %w", err)
	}
	if !payload.Success {
		return nil, fmt.Errorf("jobicy: success=false in response")
	}
	var out []RawJob
	for _, j := range payload.Jobs {
		if j.URL == "" {
			continue
		}
		remote := true
		out = append(out, RawJob{
			Title:          j.JobTitle,
			Company:        j.CompanyName,
			Description:    StripHTML(j.Description),
			Location:       firstNonEmpty(j.Geo, "Remote"),
			URL:            j.URL,
			PostedAt:       j.PubDate,
			IsRemote:       &remote,
			SalaryMin:      j.SalaryMin,
			SalaryMax:      j.SalaryMax,
			SalaryCurrency: j.SalaryCurrency,
		})
	}
	return out, nil
}

// ---- Arbeitnow ------------------------------------------------------------

type arbeitnowSource struct {
	client   *Client
	base     string
	maxPages int
	interval int
}

func (s *arbeitnowSource) Name() string         { return "arbeitnow" }
func (s *arbeitnowSource) Interval() int        { return s.interval }
func (s *arbeitnowSource) RequiresRobots() bool { return false }

func (s *arbeitnowSource) Fetch(ctx context.Context) ([]RawJob, error) {
	var out []RawJob
	seenLinks := map[string]bool{}
	next := s.base + "/api/job-board-api"
	for page := 0; page < s.maxPages && next != ""; page++ {
		if seenLinks[next] {
			return out, fmt.Errorf("arbeitnow: pagination link repeated (%s)", next)
		}
		seenLinks[next] = true
		resp, err := s.client.Get(ctx, next)
		if err != nil {
			return out, err
		}
		jobs, link, err := parseArbeitnowPage(resp.Body)
		if err != nil {
			return out, err
		}
		out = append(out, jobs...)
		next = link
	}
	return out, nil
}

func parseArbeitnowPage(body []byte) ([]RawJob, string, error) {
	var payload struct {
		Data []struct {
			Title       string          `json:"title"`
			CompanyName string          `json:"company_name"`
			Description string          `json:"description"`
			URL         string          `json:"url"`
			Location    string          `json:"location"`
			Remote      bool            `json:"remote"`
			CreatedAt   json.RawMessage `json:"created_at"`
		} `json:"data"`
		Links struct {
			Next any `json:"next"`
		} `json:"links"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, "", fmt.Errorf("arbeitnow: decode: %w", err)
	}
	var out []RawJob
	for _, j := range payload.Data {
		if j.URL == "" {
			continue
		}
		loc := j.Location
		if loc == "" && j.Remote {
			loc = "Remote"
		}
		remote := j.Remote
		out = append(out, RawJob{
			Title:       j.Title,
			Company:     j.CompanyName,
			Description: StripHTML(j.Description),
			Location:    loc,
			URL:         j.URL,
			PostedEpoch: jsonNumber(j.CreatedAt),
			PostedAt:    jsonString(j.CreatedAt),
			IsRemote:    &remote,
		})
	}
	return out, jsonStringAny(payload.Links.Next), nil
}

// ---- Working Nomads -------------------------------------------------------

type workingNomadsSource struct {
	client   *Client
	base     string
	interval int
}

func (s *workingNomadsSource) Name() string         { return "workingnomads" }
func (s *workingNomadsSource) Interval() int        { return s.interval }
func (s *workingNomadsSource) RequiresRobots() bool { return false }

func (s *workingNomadsSource) Fetch(ctx context.Context) ([]RawJob, error) {
	resp, err := s.client.Get(ctx, s.base+"/api/exposed_jobs/")
	if err != nil {
		return nil, err
	}
	return parseWorkingNomads(resp.Body)
}

func parseWorkingNomads(body []byte) ([]RawJob, error) {
	var items []struct {
		Title       string `json:"title"`
		CompanyName string `json:"company_name"`
		Description string `json:"description"`
		URL         string `json:"url"`
		Location    string `json:"location"`
		PubDate     string `json:"pub_date"`
	}
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, fmt.Errorf("workingnomads: decode: %w", err)
	}
	var out []RawJob
	for _, j := range items {
		if j.URL == "" {
			continue
		}
		remote := true
		out = append(out, RawJob{
			Title:       j.Title,
			Company:     j.CompanyName,
			Description: StripHTML(j.Description),
			Location:    firstNonEmpty(j.Location, "Remote"),
			URL:         j.URL,
			PostedAt:    j.PubDate,
			IsRemote:    &remote,
		})
	}
	return out, nil
}

// ---- JSON helpers ---------------------------------------------------------

func jsonString(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return ""
	}
	if strings.HasPrefix(s, `"`) {
		var v string
		if json.Unmarshal(raw, &v) == nil {
			return strings.TrimSpace(v)
		}
	}
	return strings.TrimSpace(s)
}

func jsonNumber(raw json.RawMessage) *float64 {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return nil
	}
	var f float64
	if json.Unmarshal(raw, &f) == nil && f > 0 {
		return &f
	}
	return nil
}

func jsonStringAny(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case nil:
		return ""
	}
	return ""
}
