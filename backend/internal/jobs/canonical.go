package jobs

import (
	"crypto/sha256"
	"net/url"
	"strings"

	"github.com/jobtool/jobtool/backend/internal/sources"
)

// Canonical is a normalised posting ready for storage.
type Canonical struct {
	Raw         sources.RawJob
	CanonicalURL string
	URLHash     []byte
	ContentHash []byte
	Title       string
}

// CanonicalizeURL strips tracking parameters, fragments and a trailing slash,
// lowercases the host and sorts the remaining query parameters.
func CanonicalizeURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if u.Host == "" {
		return "", &url.Error{Op: "canonicalize", URL: raw, Err: errNoHost}
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	u.RawFragment = ""
	q := u.Query()
	for key := range q {
		if isTrackingParam(key) {
			q.Del(key)
		}
	}
	u.RawQuery = q.Encode()
	u.Path = strings.TrimRight(u.Path, "/")
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String(), nil
}

func isTrackingParam(key string) bool {
	k := strings.ToLower(key)
	return strings.HasPrefix(k, "utm_") || k == "gh_src" || k == "lever-source" || k == "ref"
}

// URLHash is the dedupe key for a canonical URL.
func URLHash(canonicalURL string) []byte {
	sum := sha256.Sum256([]byte(canonicalURL))
	return sum[:]
}

// ContentHash is the dedupe key for the text of a posting.
func ContentHash(title, description string) []byte {
	norm := strings.ToLower(title + "\n" + description)
	norm = strings.Join(strings.Fields(norm), " ")
	sum := sha256.Sum256([]byte(norm))
	return sum[:]
}

// Normalize turns a RawJob into a Canonical posting.
func Normalize(raw sources.RawJob) (Canonical, error) {
	canon, err := CanonicalizeURL(raw.URL)
	if err != nil {
		return Canonical{}, err
	}
	return Canonical{
		Raw:          raw,
		CanonicalURL: canon,
		URLHash:      URLHash(canon),
		ContentHash:  ContentHash(raw.Title, raw.Description),
		Title:        strings.TrimSpace(raw.Title),
	}, nil
}

var errNoHost = urlError("missing host in job url")

type urlError string

func (e urlError) Error() string { return string(e) }
