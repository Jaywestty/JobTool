package ingest

import (
	"crypto/sha256"
	"strings"
)

// URLHash is the cross-source dedupe key.
func URLHash(canonicalURL string) []byte {
	sum := sha256.Sum256([]byte(canonicalURL))
	return sum[:]
}

// ContentHash is the text-level dedupe key. Normalising case and whitespace
// means the same posting re-crawled with cosmetic diffs still collides.
func ContentHash(company, title, description string) []byte {
	norm := strings.ToLower(company + "\n" + title + "\n" + description)
	norm = strings.Join(strings.Fields(norm), " ")
	sum := sha256.Sum256([]byte(norm))
	return sum[:]
}

// Fingerprints are computed once per posting.
type Fingerprints struct {
	CanonicalURL string
	URLHash      []byte
	ContentHash  []byte
}

// Fingerprint canonicalises the URL and hashes the content.
func Fingerprint(j Job) (Fingerprints, error) {
	canon, err := canonicalizeURL(j.URL)
	if err != nil {
		return Fingerprints{}, err
	}
	return Fingerprints{
		CanonicalURL: canon,
		URLHash:      URLHash(canon),
		ContentHash:  ContentHash(j.Company, j.Title, j.Description),
	}, nil
}
