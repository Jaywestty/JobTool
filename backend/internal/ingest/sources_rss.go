package ingest

import (
	"context"
	"encoding/xml"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

var defaultWWRFeeds = []string{
	"https://weworkremotely.com/remote-jobs.rss",
	"https://weworkremotely.com/categories/remote-programming-jobs.rss",
}

const (
	defaultPythonOrgBase = "https://www.python.org"
	browserUserAgent     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36"
)

// ---- We Work Remotely -----------------------------------------------------

type wwrSource struct {
	client   *Client
	feeds    []string
	interval int
	log      *slog.Logger
}

func (s *wwrSource) Name() string         { return "weworkremotely" }
func (s *wwrSource) Interval() int        { return s.interval }
func (s *wwrSource) RequiresRobots() bool { return false }

func (s *wwrSource) Fetch(ctx context.Context) ([]RawJob, error) {
	var out []RawJob
	for _, feed := range s.feeds {
		resp, err := s.client.GetWithHeaders(ctx, feed, map[string]string{"User-Agent": browserUserAgent})
		if err != nil {
			if se := statusOf(err); se != 0 && se != 200 {
				s.log.Error("weworkremotely feed returned non-200", "feed", feed, "status", se)
			}
			return out, err
		}
		jobs, err := parseWWR(resp.Body)
		if err != nil {
			return out, err
		}
		out = append(out, jobs...)
	}
	return out, nil
}

type rssFeed struct {
	Channel struct {
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	Region      string `xml:"region"`
	Creator     string `xml:"http://purl.org/dc/elements/1.1/ creator"`
	CreatorBare string `xml:"creator"`
}

func parseWWR(body []byte) ([]RawJob, error) {
	var feed rssFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("weworkremotely: decode rss: %w", err)
	}
	var out []RawJob
	for _, it := range feed.Channel.Items {
		u := firstNonEmpty(it.Link, it.GUID)
		if u == "" {
			continue
		}
		company := firstNonEmpty(it.Creator, it.CreatorBare)
		title := it.Title
		if company == "" {
			company, title = splitCompanyTitle(title)
		}
		remote := true
		out = append(out, RawJob{
			Title:       title,
			Company:     company,
			Description: StripHTML(it.Description),
			Location:    firstNonEmpty(it.Region, "Remote"),
			URL:         u,
			PostedAt:    it.PubDate,
			IsRemote:    &remote,
		})
	}
	return out, nil
}

// ---- python.org Jobs ------------------------------------------------------

type pythonOrgSource struct {
	client   *Client
	base     string
	interval int
	now      func() time.Time
}

func (s *pythonOrgSource) Name() string         { return "pythonorg" }
func (s *pythonOrgSource) Interval() int        { return s.interval }
func (s *pythonOrgSource) RequiresRobots() bool { return false }

func (s *pythonOrgSource) nowUTC() time.Time {
	if s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

func (s *pythonOrgSource) Fetch(ctx context.Context) ([]RawJob, error) {
	resp, err := s.client.GetWithHeaders(ctx, s.base+"/jobs/feed/rss/", map[string]string{"User-Agent": browserUserAgent})
	if err != nil {
		return nil, err
	}
	return parsePythonOrg(resp.Body, s.nowUTC())
}

func parsePythonOrg(body []byte, firstSeen time.Time) ([]RawJob, error) {
	var feed rssFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("pythonorg: decode rss: %w", err)
	}
	stamp := firstSeen.UTC().Format(time.RFC3339)
	var out []RawJob
	for _, it := range feed.Channel.Items {
		u := firstNonEmpty(it.Link, it.GUID)
		if u == "" {
			continue
		}
		title, company := splitTitleLastComma(it.Title)
		if title == "" {
			continue
		}
		rawDesc := it.Description
		location := FirstLine(rawDesc)
		description := StripHTML(rawDesc)
		if location != "" && strings.HasPrefix(description, location) {
			description = strings.TrimSpace(description[len(location):])
		}
		remote := strings.Contains(strings.ToLower(location), "remote")
		out = append(out, RawJob{
			Title:       title,
			Company:     company,
			Description: description,
			Location:    firstNonEmpty(location, "Remote"),
			URL:         u,
			PostedAt:    stamp,
			IsRemote:    &remote,
		})
	}
	return out, nil
}

func splitTitleLastComma(raw string) (title, company string) {
	text := strings.TrimSpace(raw)
	if i := strings.LastIndex(text, ","); i >= 0 {
		return strings.TrimSpace(text[:i]), strings.TrimSpace(text[i+1:])
	}
	return text, ""
}

// splitCompanyTitle splits the "Company: Role" titles used by some feeds. A
// colon only splits when the left side is short enough to be a company name.
func splitCompanyTitle(raw string) (company, title string) {
	for _, sep := range []string{" : ", ": "} {
		if i := strings.Index(raw, sep); i > 0 && i <= 60 {
			return strings.TrimSpace(raw[:i]), strings.TrimSpace(raw[i+len(sep):])
		}
	}
	return "", strings.TrimSpace(raw)
}

// ---- Hacker News "Who is hiring?" ----------------------------------------

const (
	defaultHNSearchBase = "https://hn.algolia.com/api/v1"
	defaultHNItemBase   = "https://hn.algolia.com/api/v1"
	hiringTitlePrefix   = "ask hn: who is hiring?"
)

type hackerNewsSource struct {
	client     *Client
	searchBase string
	itemBase   string
	monthsBack int
	interval   int
	now        func() time.Time
}

func (s *hackerNewsSource) Name() string         { return "hackernews" }
func (s *hackerNewsSource) Interval() int        { return s.interval }
func (s *hackerNewsSource) RequiresRobots() bool { return false }

func (s *hackerNewsSource) nowUTC() time.Time {
	if s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

func (s *hackerNewsSource) Fetch(ctx context.Context) ([]RawJob, error) {
	cutoff := s.nowUTC().AddDate(0, -s.monthsBack, 0)
	storyID, title, err := s.findThread(ctx, cutoff)
	if err != nil {
		return nil, err
	}
	if storyID == 0 {
		return nil, nil
	}
	resp, err := s.client.Get(ctx, fmt.Sprintf("%s/items/%d", s.itemBase, storyID))
	if err != nil {
		return nil, err
	}
	return parseHNThread(resp.Body, storyID, title)
}

func (s *hackerNewsSource) findThread(ctx context.Context, cutoff time.Time) (int64, string, error) {
	u := s.searchBase + "/search_by_date?tags=story,author_whoishiring&hitsPerPage=30"
	resp, err := s.client.Get(ctx, u)
	if err != nil {
		return 0, "", err
	}
	return parseHNThreadSearch(resp.Body, cutoff)
}

func parseHNThreadSearch(body []byte, cutoff time.Time) (int64, string, error) {
	var payload struct {
		Hits []struct {
			ObjectID  jsonNumberAlias `json:"objectID"`
			Title     string          `json:"title"`
			CreatedAt string          `json:"created_at"`
		} `json:"hits"`
	}
	if err := jsonUnmarshal(body, &payload); err != nil {
		return 0, "", fmt.Errorf("hackernews: decode search: %w", err)
	}
	var bestID int64
	var bestTitle string
	var best time.Time
	for _, hit := range payload.Hits {
		low := strings.ToLower(strings.TrimSpace(hit.Title))
		if !strings.HasPrefix(low, hiringTitlePrefix) || strings.Contains(low, "freelanc") {
			continue
		}
		created, _ := NormalizeTimestamp(hit.CreatedAt)
		if created == nil || created.Before(cutoff) {
			continue
		}
		id, _ := hit.ObjectID.Int64()
		if id == 0 {
			continue
		}
		if best.IsZero() || created.After(best) {
			best = *created
			bestID = id
			bestTitle = hit.Title
		}
	}
	return bestID, bestTitle, nil
}

func parseHNThread(body []byte, storyID int64, title string) ([]RawJob, error) {
	var item struct {
		Children []hnComment `json:"children"`
	}
	if err := jsonUnmarshal(body, &item); err != nil {
		return nil, fmt.Errorf("hackernews: decode item: %w", err)
	}
	var out []RawJob
	for _, c := range item.Children {
		if c.Deleted || c.Dead {
			continue
		}
		if c.ParentID != nil && *c.ParentID != storyID {
			continue
		}
		job, ok := hnCommentToJob(c)
		if ok {
			out = append(out, job)
		}
	}
	return out, nil
}

type hnComment struct {
	ID        *int64 `json:"id"`
	ParentID  *int64 `json:"parent_id"`
	Text      string `json:"text"`
	CreatedAt string `json:"created_at"`
	Deleted   bool   `json:"deleted"`
	Dead      bool   `json:"dead"`
}

func hnCommentToJob(c hnComment) (RawJob, bool) {
	if c.ID == nil {
		return RawJob{}, false
	}
	raw := strings.TrimSpace(c.Text)
	if raw == "" {
		return RawJob{}, false
	}
	firstLine := FirstLine(raw)
	body := StripHTML(raw)
	parts := splitNonEmpty(firstLine, "|")
	var company, title, location string
	switch {
	case len(parts) >= 2:
		company, title = parts[0], parts[1]
		if len(parts) >= 3 {
			location = parts[2]
		}
	case len(parts) == 1:
		title = truncate(parts[0], 120)
	default:
		title = truncate(body, 120)
	}
	if strings.TrimSpace(title) == "" {
		return RawJob{}, false
	}
	remote := wholeWord("remote").MatchString(firstLine) ||
		wholeWord("distributed").MatchString(firstLine)
	desc := firstLine
	if body != "" {
		desc = firstLine + "\n" + truncate(body, 400)
	}
	if location == "" && remote {
		location = "Remote"
	}
	return RawJob{
		Title:       title,
		Company:     company,
		Description: truncate(desc, 4000),
		Location:    location,
		URL:         fmt.Sprintf("https://news.ycombinator.com/item?id=%d", *c.ID),
		PostedAt:    c.CreatedAt,
		IsRemote:    &remote,
	}, true
}

func splitNonEmpty(s, sep string) []string {
	var out []string
	for _, p := range strings.Split(s, sep) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
