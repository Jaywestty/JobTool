package ingest

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// testClient builds a client pointing at a test server with no spacing delay.
func testClient() *Client {
	return NewClient(ClientOptions{UserAgent: "jobtool-test/1.0", Timeout: 5 * time.Second, MinGap: 0, Retries: 0})
}

func fixture(handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(handler)
}

// ---- Greenhouse -----------------------------------------------------------

func TestGreenhouseFixture(t *testing.T) {
	body := `{"jobs":[
		{"id":1,"title":"Backend Engineer","absolute_url":"https://acme.test/jobs/1","content":"<p>Go and <b>Postgres</b></p>","updated_at":"2026-10-01T10:00:00-04:00","location":{"name":"Remote - US"}},
		{"id":2,"title":"No URL","absolute_url":"","content":"x","location":{"name":"Berlin"}},
		{"id":3,"title":"ML Engineer","absolute_url":"https://acme.test/jobs/3","content":"<p>PyTorch</p>","updated_at":"2026-10-02T00:00:00Z","offices":[{"name":"London"}]}
	]}`
	srv := fixture(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/boards/acme" || r.URL.Path == "/v1/boards/acme/jobs" {
			_, _ = io.WriteString(w, body)
			return
		}
		http.NotFound(w, r)
	})
	defer srv.Close()

	s := &greenhouseSource{slug: "acme", client: testClient(), base: srv.URL, interval: 60}
	jobs, err := s.Fetch(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs want 2 (empty url dropped)", len(jobs))
	}
	if jobs[0].Description != "Go and Postgres" {
		t.Fatalf("html not stripped: %q", jobs[0].Description)
	}
	if jobs[1].Location != "London" {
		t.Fatalf("office fallback failed: %q", jobs[1].Location)
	}
	if err := s.Validate(context.Background()); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestGreenhouseDeadSlug(t *testing.T) {
	srv := fixture(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	defer srv.Close()
	s := &greenhouseSource{slug: "dead", client: testClient(), base: srv.URL}
	if err := s.Validate(context.Background()); !isDeadSlug(err) {
		t.Fatalf("expected dead slug, got %v", err)
	}
}

// ---- Lever ----------------------------------------------------------------

func TestLeverFixture(t *testing.T) {
	body := `[
		{"text":"Platform Engineer","hostedUrl":"https://jobs.lever.co/acme/1","description":"<div>Kubernetes</div>","createdAt":1759300000000,"workplaceType":"remote","categories":{"location":"Europe","commitment":"Full-time"}},
		{"text":"Designer","hostedUrl":"","description":"x"}
	]`
	srv := fixture(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) })
	defer srv.Close()

	s := &leverSource{slug: "acme", client: testClient(), base: srv.URL}
	jobs, err := s.Fetch(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs want 1", len(jobs))
	}
	if jobs[0].PostedAt != "1759300000000" {
		t.Fatalf("epoch millis not preserved: %q", jobs[0].PostedAt)
	}
	if jobs[0].IsRemote == nil || !*jobs[0].IsRemote {
		t.Fatal("remote workplace not detected")
	}
}

// ---- Ashby ----------------------------------------------------------------

func TestAshbyFixture(t *testing.T) {
	body := `{"jobs":[
		{"title":"Security Engineer","jobUrl":"https://jobs.ashbyhq.com/acme/1","descriptionHtml":"<p>AppSec</p>","location":"Remote","isRemote":true,"publishedAt":"2026-09-30T12:00:00Z","compensation":{"compensationTiers":[{"min":150000,"max":200000}]}},
		{"title":"","jobUrl":"https://jobs.ashbyhq.com/acme/2"},
		{"title":"Data Engineer","jobUrl":"https://jobs.ashbyhq.com/acme/3","descriptionHtml":"","descriptionPlain":"dbt pipelines","location":"","isRemote":false,"secondaryLocations":["Berlin, DE"],"compensation":{"compensationTierSummary":"EUR 90,000 - 120,000"}}
	]}`
	srv := fixture(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) })
	defer srv.Close()

	s := &ashbySource{slug: "acme", client: testClient(), base: srv.URL}
	jobs, err := s.Fetch(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs want 2", len(jobs))
	}
	if jobs[0].SalaryMin == nil || *jobs[0].SalaryMin != 150000 {
		t.Fatalf("tier salary not parsed: %v", jobs[0].SalaryMin)
	}
	if !strings.Contains(jobs[1].Location, "Berlin") {
		t.Fatalf("secondary location missing: %q", jobs[1].Location)
	}
	if !strings.Contains(jobs[1].Description, "dbt") {
		t.Fatalf("plain description fallback failed: %q", jobs[1].Description)
	}
}

// ---- RemoteOK -------------------------------------------------------------

func TestRemoteOKFixture(t *testing.T) {
	body := `[
		{"legal":"metadata element, not a job"},
		{"position":"Backend Engineer","company":"Acme","description":"Go services","url":"https://remoteok.com/j/1","location":"Worldwide","epoch":1759300000,"salary_min":90000,"salary_max":120000},
		{"position":"No URL","company":"Acme"}
	]`
	srv := fixture(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) })
	defer srv.Close()

	s := &remoteOKSource{client: testClient(), base: srv.URL}
	jobs, err := s.Fetch(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs want 1 (metadata skipped)", len(jobs))
	}
	if jobs[0].PostedEpoch == nil || *jobs[0].PostedEpoch != 1759300000 {
		t.Fatal("epoch not preserved")
	}
}

// ---- Remotive -------------------------------------------------------------

func TestRemotiveFixture(t *testing.T) {
	srv := fixture(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("search") != "python" {
			t.Errorf("unexpected search %q", r.URL.Query().Get("search"))
		}
		_, _ = io.WriteString(w, `{"jobs":[{"title":"Python Developer","company_name":"Acme","description":"<p>Django</p>","url":"https://remotive.test/1","candidate_required_location":"Worldwide","publication_date":"2026-10-01T08:00:00Z"}]}`)
	})
	defer srv.Close()

	s := &remotiveSource{client: testClient(), base: srv.URL, searches: []string{"python"}}
	jobs, err := s.Fetch(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Location != "Worldwide" {
		t.Fatalf("unexpected jobs: %+v", jobs)
	}
}

// ---- Himalayas ------------------------------------------------------------

func TestHimalayasCursorAndRepeatGuard(t *testing.T) {
	calls := 0
	srv := fixture(func(w http.ResponseWriter, r *http.Request) {
		calls++
		cursor := r.URL.Query().Get("cursor")
		switch cursor {
		case "":
			_, _ = io.WriteString(w, `{"jobs":[{"title":"Backend Engineer","companyName":"Acme","description":"Go","applicationLink":"https://h.test/1","pubDate":1759300000}],"nextCursor":"c1"}`)
		case "c1":
			_, _ = io.WriteString(w, `{"jobs":[{"title":"Data Engineer","companyName":"Acme","description":"ETL","applicationLink":"https://h.test/2","pubDate":1759300100}],"nextCursor":"c2"}`)
		case "c2":
			_, _ = io.WriteString(w, `{"jobs":[],"nextCursor":"c1"}`)
		default:
			http.Error(w, "bad cursor", http.StatusBadRequest)
		}
	})
	defer srv.Close()

	s := &himalayasSource{client: testClient(), base: srv.URL, maxPages: 10}
	jobs, err := s.Fetch(context.Background())
	if err == nil {
		t.Fatal("expected repeated-cursor error")
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs want 2 before the guard", len(jobs))
	}
	if calls != 3 {
		t.Fatalf("calls = %d want 3", calls)
	}
}

func TestHimalayasPageCap(t *testing.T) {
	calls := 0
	srv := fixture(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = fmt.Fprintf(w, `{"jobs":[{"title":"Backend Engineer","companyName":"Acme","description":"Go","applicationLink":"https://h.test/%d","pubDate":1759300000}],"nextCursor":"c%d"}`, calls, calls)
	})
	defer srv.Close()

	s := &himalayasSource{client: testClient(), base: srv.URL, maxPages: 3}
	jobs, err := s.Fetch(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if calls != 3 || len(jobs) != 3 {
		t.Fatalf("calls=%d jobs=%d want 3 and 3", calls, len(jobs))
	}
}

// ---- Jobicy ---------------------------------------------------------------

func TestJobicyFixture(t *testing.T) {
	srv := fixture(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("count") != "50" || r.URL.Query().Get("industry") != "engineering" {
			t.Errorf("unexpected query %q", r.URL.RawQuery)
		}
		_, _ = io.WriteString(w, `{"success":true,"jobs":[{"jobTitle":"DevOps Engineer","companyName":"Acme","jobDescription":"<p>Terraform</p>","url":"https://jobicy.test/1","jobGeo":"Anywhere","pubDate":"2026-10-01T00:00:00Z","salaryMin":100000,"salaryMax":140000,"salaryCurrency":"USD"}]}`)
	})
	defer srv.Close()

	s := &jobicySource{client: testClient(), base: srv.URL, count: 50}
	jobs, err := s.Fetch(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].SalaryCurrency != "USD" {
		t.Fatalf("unexpected jobs: %+v", jobs)
	}
}

func TestJobicyErrorResponse(t *testing.T) {
	srv := fixture(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, `{"success":false}`) })
	defer srv.Close()
	s := &jobicySource{client: testClient(), base: srv.URL, count: 50}
	if _, err := s.Fetch(context.Background()); err == nil {
		t.Fatal("expected error on success=false")
	}
}

// ---- We Work Remotely -----------------------------------------------------

func TestWWWRSRFC822(t *testing.T) {
	feed := `<?xml version="1.0"?><rss><channel>
		<item><title>Acme : Senior Backend Engineer</title><link>https://wwr.test/1</link>
		<description>&lt;p&gt;Go and Postgres&lt;/p&gt;</description>
		<pubDate>Thu, 01 Oct 2026 12:00:00 +0000</pubDate><region>Remote</region></item>
	</channel></rss>`
	srv := fixture(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.UserAgent(), "Mozilla") {
			t.Errorf("expected browser user agent, got %q", r.UserAgent())
		}
		_, _ = io.WriteString(w, feed)
	})
	defer srv.Close()

	s := &wwrSource{client: testClient(), feeds: []string{srv.URL + "/remote-jobs.rss"}, log: nopLogger()}
	jobs, err := s.Fetch(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs", len(jobs))
	}
	if jobs[0].Company != "Acme" || jobs[0].Title != "Senior Backend Engineer" {
		t.Fatalf("company:title split failed: %+v", jobs[0])
	}
	got, _ := NormalizeTimestamp(jobs[0].PostedAt)
	if got == nil || !got.Equal(time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("rfc822 not parsed: %v", got)
	}
}

func TestWWWRLoudOnNon200(t *testing.T) {
	srv := fixture(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) })
	defer srv.Close()
	s := &wwrSource{client: testClient(), feeds: []string{srv.URL + "/feed.rss"}, log: nopLogger()}
	if _, err := s.Fetch(context.Background()); err == nil {
		t.Fatal("expected error on 403 (not retried)")
	}
}

// ---- Arbeitnow ------------------------------------------------------------

func TestArbeitnowPaging(t *testing.T) {
	srv := fixture(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "" {
			_, _ = fmt.Fprintf(w, `{"data":[{"title":"Backend Engineer","company_name":"Acme","description":"Go","url":"https://a.test/1","location":"","remote":true,"created_at":1759300000}],"links":{"next":"%s/api/job-board-api?page=2"}}`, "http://"+r.Host)
			return
		}
		_, _ = io.WriteString(w, `{"data":[{"title":"Data Engineer","company_name":"Acme","description":"ETL","url":"https://a.test/2","location":"Berlin","remote":false,"created_at":1759300100}],"links":{"next":null}}`)
	})
	defer srv.Close()

	s := &arbeitnowSource{client: testClient(), base: srv.URL, maxPages: 5}
	jobs, err := s.Fetch(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs want 2", len(jobs))
	}
	if jobs[0].Location != "Remote" {
		t.Fatalf("remote location default failed: %q", jobs[0].Location)
	}
}

// ---- Working Nomads -------------------------------------------------------

func TestWorkingNomadsFixture(t *testing.T) {
	srv := fixture(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[{"title":"Mobile Engineer","company_name":"Acme","description":"<p>Flutter</p>","url":"https://wn.test/1","location":"Anywhere","pub_date":"2026-10-01T00:00:00+02:00"}]`)
	})
	defer srv.Close()
	s := &workingNomadsSource{client: testClient(), base: srv.URL}
	jobs, err := s.Fetch(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Title != "Mobile Engineer" {
		t.Fatalf("unexpected: %+v", jobs)
	}
}

// ---- python.org -----------------------------------------------------------

func TestPythonOrgFirstSeen(t *testing.T) {
	feed := `<?xml version="1.0"?><rss><channel>
		<item><title>Backend Developer, Example Corp</title><link>https://python.org/jobs/1</link>
		<description>Remote, USA&lt;p&gt;Django and Python&lt;/p&gt;</description></item>
	</channel></rss>`
	srv := fixture(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, feed) })
	defer srv.Close()

	fixed := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	s := &pythonOrgSource{client: testClient(), base: srv.URL, now: func() time.Time { return fixed }}
	jobs, err := s.Fetch(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs", len(jobs))
	}
	if jobs[0].Company != "Example Corp" || jobs[0].Title != "Backend Developer" {
		t.Fatalf("last-comma split failed: %q / %q", jobs[0].Company, jobs[0].Title)
	}
	if jobs[0].Location != "Remote, USA" {
		t.Fatalf("first line location failed: %q", jobs[0].Location)
	}
	posted, _ := NormalizeTimestamp(jobs[0].PostedAt)
	if posted == nil || !posted.Equal(fixed) {
		t.Fatalf("expected first-seen time, got %v", posted)
	}
}

// ---- Hacker News ----------------------------------------------------------

func TestHackerNewsThreadAndChildren(t *testing.T) {
	searchBody := `{"hits":[
		{"objectID":100,"title":"Ask HN: Who wants to be hired?","created_at":"2026-10-01T00:00:00Z"},
		{"objectID":200,"title":"Ask HN: Who is hiring? (October 2026)","created_at":"2026-10-01T00:00:00Z"},
		{"objectID":150,"title":"Ask HN: Who is hiring? (September 2026)","created_at":"2026-09-01T00:00:00Z"}
	]}`
	itemBody := `{"id":200,"children":[
		{"id":201,"parent_id":200,"text":"Acme | Backend Engineer | Remote | We use Go","created_at":"2026-10-01T01:00:00Z"},
		{"id":202,"parent_id":200,"text":"","created_at":"2026-10-01T02:00:00Z"},
		{"id":203,"parent_id":200,"deleted":true,"text":"x"},
		{"id":204,"parent_id":999,"text":"Nested reply | not a posting","created_at":"2026-10-01T03:00:00Z"},
		{"id":205,"parent_id":200,"text":"Acme | Data Engineer | Berlin","created_at":"2026-10-01T04:00:00Z"}
	]}`
	srv := fixture(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "search_by_date"):
			_, _ = io.WriteString(w, searchBody)
		case strings.HasSuffix(r.URL.Path, "/items/200"):
			_, _ = io.WriteString(w, itemBody)
		default:
			http.NotFound(w, r)
		}
	})
	defer srv.Close()

	s := &hackerNewsSource{client: testClient(), searchBase: srv.URL, itemBase: srv.URL, monthsBack: 2, now: func() time.Time { return time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC) }}
	jobs, err := s.Fetch(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs want 2 (deleted, empty and nested excluded)", len(jobs))
	}
	if jobs[0].Company != "Acme" || jobs[0].Title != "Backend Engineer" {
		t.Fatalf("pipe split failed: %+v", jobs[0])
	}
	if jobs[0].URL != "https://news.ycombinator.com/item?id=201" {
		t.Fatalf("unexpected url %q", jobs[0].URL)
	}
}

// ---- helpers --------------------------------------------------------------

func nopLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
