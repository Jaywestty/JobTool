package jobs

import (
	"bytes"
	"testing"

	"github.com/jobtool/jobtool/backend/internal/sources"
)

func TestCanonicalizeURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "strips utm and fragment",
			in:   "https://Boards.Greenhouse.io/acme/jobs/1?utm_source=x&gh_src=y#apply",
			want: "https://boards.greenhouse.io/acme/jobs/1",
		},
		{
			name: "sorts query params",
			in:   "https://example.com/jobs?b=2&a=1",
			want: "https://example.com/jobs?a=1&b=2",
		},
		{
			name: "trailing slash removed",
			in:   "https://example.com/jobs/",
			want: "https://example.com/jobs",
		},
		{
			name: "keeps meaningful query",
			in:   "https://example.com/jobs?id=42&utm_medium=email",
			want: "https://example.com/jobs?id=42",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CanonicalizeURL(tt.in)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestCanonicalizeRejectsRelative(t *testing.T) {
	if _, err := CanonicalizeURL("/jobs/1"); err == nil {
		t.Fatal("expected an error for a relative url")
	}
}

func TestNormalizeDedupeKeys(t *testing.T) {
	a := sources.RawJob{URL: "https://example.com/jobs/1?utm_source=a", Title: "Senior  Go Engineer", Description: "Build services."}
	b := sources.RawJob{URL: "https://example.com/jobs/1", Title: "Senior Go Engineer", Description: "Build services."}

	na, err := Normalize(a)
	if err != nil {
		t.Fatal(err)
	}
	nb, err := Normalize(b)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(na.URLHash, nb.URLHash) {
		t.Fatal("same canonical url should give the same url hash")
	}
	if !bytes.Equal(na.ContentHash, nb.ContentHash) {
		t.Fatal("whitespace differences should not change the content hash")
	}

	c := sources.RawJob{URL: "https://example.com/jobs/2", Title: "Different role", Description: "Something else."}
	nc, err := Normalize(c)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(na.URLHash, nc.URLHash) {
		t.Fatal("different urls should not collide")
	}
	if bytes.Equal(na.ContentHash, nc.ContentHash) {
		t.Fatal("different content should not collide")
	}
}
