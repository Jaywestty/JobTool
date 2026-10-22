package sources

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func mustReader(s string) io.Reader { return strings.NewReader(s) }

const greenhouseFixture = `{
  "jobs": [
    {
      "id": 123,
      "title": "Senior Backend Engineer",
      "absolute_url": "https://boards.greenhouse.io/acme/jobs/123",
      "content": "We use Go and Postgres.",
      "updated_at": "2026-10-01T12:00:00-04:00",
      "location": {"name": "Remote - EU"}
    },
    {
      "id": 124,
      "title": "ML Engineer",
      "absolute_url": "https://boards.greenhouse.io/acme/jobs/124",
      "content": "PyTorch and Python.",
      "updated_at": "not-a-date",
      "location": {"name": "Berlin"}
    }
  ]
}`

func TestParseGreenhouseFixture(t *testing.T) {
	jobs, err := ParseGreenhouse(mustReader(greenhouseFixture), "acme")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs", len(jobs))
	}
	tests := []struct {
		idx      int
		extID    string
		source   string
		postedOK bool
	}{
		{idx: 0, extID: "123", source: "greenhouse:acme", postedOK: true},
		{idx: 1, extID: "124", source: "greenhouse:acme", postedOK: false},
	}
	for _, tt := range tests {
		got := jobs[tt.idx]
		if got.ExternalID != tt.extID {
			t.Errorf("job %d external id = %q", tt.idx, got.ExternalID)
		}
		if got.Source != tt.source {
			t.Errorf("job %d source = %q", tt.idx, got.Source)
		}
		if (got.PostedAt != nil) != tt.postedOK {
			t.Errorf("job %d postedAt present = %v", tt.idx, got.PostedAt != nil)
		}
	}
}

func TestGreenhouseFetchFromServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/boards/acme/jobs" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(greenhouseFixture))
	}))
	defer srv.Close()

	// Point the source at the test server. The handler asserts the real path.
	client := srv.Client()
	g := Greenhouse{Board: "acme", BaseURL: srv.URL, Client: client}
	jobs, err := g.Fetch(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs", len(jobs))
	}
}
