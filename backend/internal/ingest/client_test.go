package ingest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientRetriesOnlyTransient(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		wantCalls int32
		wantErr   bool
	}{
		{name: "200 no retry", status: 200, wantCalls: 1},
		{name: "400 no retry", status: 400, wantCalls: 1, wantErr: true},
		{name: "401 no retry", status: 401, wantCalls: 1, wantErr: true},
		{name: "403 no retry", status: 403, wantCalls: 1, wantErr: true},
		{name: "404 no retry", status: 404, wantCalls: 1, wantErr: true},
		{name: "429 retries", status: 429, wantCalls: 3, wantErr: true},
		{name: "500 retries", status: 500, wantCalls: 3, wantErr: true},
		{name: "503 retries", status: 503, wantCalls: 3, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&calls, 1)
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, "x")
			}))
			defer srv.Close()

			c := NewClient(ClientOptions{UserAgent: "jobtool-test/1.0", Timeout: 2 * time.Second, MinGap: 0, Retries: 2, Backoff: time.Millisecond})
			_, err := c.Get(context.Background(), srv.URL)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v wantErr %v", err, tt.wantErr)
			}
			if got := atomic.LoadInt32(&calls); got != tt.wantCalls {
				t.Fatalf("calls = %d want %d", got, tt.wantCalls)
			}
		})
	}
}

func TestClientSendsUserAgent(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.UserAgent()
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()
	c := NewClient(ClientOptions{UserAgent: "jobtool/9.9 (+https://example.test)", MinGap: 0, Timeout: 2 * time.Second})
	if _, err := c.Get(context.Background(), srv.URL); err != nil {
		t.Fatal(err)
	}
	if seen != "jobtool/9.9 (+https://example.test)" {
		t.Fatalf("user agent = %q", seen)
	}
}

func TestParseRobotsAndAllow(t *testing.T) {
	text := `
User-agent: *
Disallow: /private
Allow: /private/ok

User-agent: jobtool
Disallow: /
Allow: /jobs
`
	rb := ParseRobots(text)
	tests := []struct {
		agent string
		path  string
		want  bool
	}{
		{agent: "jobtool/1.0", path: "/jobs/1", want: true},
		{agent: "jobtool/1.0", path: "/private", want: false},
		{agent: "other", path: "/private", want: false},
		{agent: "other", path: "/private/ok", want: true},
		{agent: "other", path: "/public", want: true},
	}
	for _, tt := range tests {
		if got := rb.Allowed(tt.agent, tt.path); got != tt.want {
			t.Fatalf("Allowed(%q,%q) = %v want %v", tt.agent, tt.path, got, tt.want)
		}
	}
}

func TestRobotsAllowedFailsOpen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()
	c := NewClient(ClientOptions{UserAgent: "jobtool/1.0", MinGap: 0, Timeout: 2 * time.Second})
	if !c.robots.Allowed(context.Background(), srv.URL+"/anything") {
		t.Fatal("missing robots.txt should allow")
	}
}

func TestRegistrySlugValidationDropsDead(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/boards/live" || r.URL.Path == "/v0/postings/live" || r.URL.Path == "/posting-api/job-board/live" {
			_, _ = io.WriteString(w, `{"jobs":[]}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	cfg := &Config{}
	cfg.Sources.ATS.Greenhouse = ATSSlugs{Enabled: true, IntervalSeconds: 60, Slugs: []string{"dead"}}
	cfg.Sources.ATS.Lever = ATSSlugs{Enabled: true, IntervalSeconds: 60, Slugs: []string{"dead"}}
	cfg.Sources.ATS.Ashby = ATSSlugs{Enabled: true, IntervalSeconds: 60, Slugs: []string{"dead"}}
	client := testClient()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	// Build with explicit bases so the test server is used.
	r := &Registry{sources: map[string]Source{}, log: log}
	r.add("greenhouse:dead", &greenhouseSource{slug: "dead", client: client, base: srv.URL})
	r.add("lever:dead", &leverSource{slug: "dead", client: client, base: srv.URL})
	r.add("ashby:dead", &ashbySource{slug: "dead", client: client, base: srv.URL})
	r.ValidateSlugs(context.Background())
	if len(r.Names()) != 0 {
		t.Fatalf("dead slugs not dropped: %v", r.Names())
	}
}

var _ = errors.Is
