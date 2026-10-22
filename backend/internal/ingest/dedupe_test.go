package ingest

import (
	"bytes"
	"testing"
	"time"
)

func TestFingerprintDedupeAcrossSources(t *testing.T) {
	a := Job{Title: "Senior  Go Engineer", Company: "Acme", Description: "Build services.", URL: "https://example.com/jobs/1?utm_source=greenhouse#apply"}
	b := Job{Title: "senior go engineer", Company: "acme", Description: "Build   services.", URL: "https://example.com/jobs/1?ref=lever"}

	fa, err := Fingerprint(a)
	if err != nil {
		t.Fatal(err)
	}
	fb, err := Fingerprint(b)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fa.URLHash, fb.URLHash) {
		t.Fatal("same canonical url across sources should share a url hash")
	}
	if !bytes.Equal(fa.ContentHash, fb.ContentHash) {
		t.Fatal("cosmetic text differences should share a content hash")
	}
	if fa.CanonicalURL != "https://example.com/jobs/1" {
		t.Fatalf("canonical url = %q", fa.CanonicalURL)
	}
}

func TestFingerprintDifferentJobs(t *testing.T) {
	a := Job{Title: "Backend Engineer", Company: "Acme", URL: "https://example.com/jobs/1"}
	b := Job{Title: "Frontend Engineer", Company: "Acme", URL: "https://example.com/jobs/2"}
	fa, _ := Fingerprint(a)
	fb, _ := Fingerprint(b)
	if bytes.Equal(fa.URLHash, fb.URLHash) || bytes.Equal(fa.ContentHash, fb.ContentHash) {
		t.Fatal("different jobs must not collide")
	}
}

func TestFingerprintRejectsRelative(t *testing.T) {
	if _, err := Fingerprint(Job{URL: "/jobs/1"}); err == nil {
		t.Fatal("expected error for a relative url")
	}
}

func TestServiceDedupesWithinRun(t *testing.T) {
	cfg := testConfig()
	norm := newNormalizer(cfg, func() time.Time { return time.Unix(1790856000, 0) })
	raw := []RawJob{
		{Title: "Backend Engineer", Company: "Acme", Description: "Build things.", URL: "https://example.com/1?utm_source=a", IsRemote: ptrBool(true)},
		{Title: "Backend Engineer", Company: "Acme", Description: "Build things.", URL: "https://example.com/1?ref=b", IsRemote: ptrBool(true)},
		{Title: "AI Engineer", Company: "Acme", Description: "Train machine learning models.", URL: "https://example.com/2", IsRemote: ptrBool(true)},
		{Title: "Sales Manager", Company: "Acme", Description: "Sell.", URL: "https://example.com/3"},
	}
	var kept int
	urlSeen := map[string]bool{}
	for _, r := range raw {
		job, ok := norm.Normalize("test", r)
		if !ok {
			continue
		}
		kept++
		fp, _ := Fingerprint(job)
		urlSeen[string(fp.URLHash)] = true
	}
	if kept != 3 {
		t.Fatalf("kept = %d want 3 (sales should be gated)", kept)
	}
	if len(urlSeen) != 2 {
		t.Fatalf("distinct urls = %d want 2 (the duplicate should collapse)", len(urlSeen))
	}
}

func ptrBool(b bool) *bool { return &b }
