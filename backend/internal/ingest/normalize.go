package ingest

import (
	"context"
	"net/url"
	"strings"
	"time"
)

// normalizer turns RawJob rows into fully classified Job rows.
type normalizer struct {
	class *classifier
	cfg   *Config
	now   func() time.Time
}

func newNormalizer(cfg *Config, now func() time.Time) *normalizer {
	if now == nil {
		now = time.Now
	}
	return &normalizer{class: newClassifier(cfg), cfg: cfg, now: now}
}

// Normalize classifies one RawJob. ok is false when the posting must be
// dropped (no title, no url, tech gate miss, non-English).
func (n *normalizer) Normalize(source string, raw RawJob) (Job, bool) {
	title := strings.TrimSpace(raw.Title)
	u := strings.TrimSpace(raw.URL)
	if title == "" || u == "" {
		return Job{}, false
	}
	desc := StripHTML(raw.Description)
	location := strings.TrimSpace(raw.Location)

	roleFamily := n.class.classifyRoleFamily(title, desc)
	if n.cfg.Gate.Enabled && roleFamily == "" {
		return Job{}, false
	}
	if ok, _ := n.class.englishOK(title, desc); !ok {
		return Job{}, false
	}

	isRemote := false
	if raw.IsRemote != nil {
		isRemote = *raw.IsRemote
	}
	isRemote = n.class.IsRemotePosting(title, location, isRemote)
	scope := n.class.ClassifyRemoteScope(title, location, desc, isRemote)
	if scope == "" && isRemote {
		scope = ScopeUnknown
	}

	posted := postedTime(raw)
	smin, smax, cur := ParseSalary("", raw.SalaryMin, raw.SalaryMax, n.cfg.Salary.Currencies)
	if smin == nil && smax == nil {
		if mn, mx, c := ParseSalary(desc, nil, nil, n.cfg.Salary.Currencies); mn != nil {
			smin, smax = mn, mx
			cur = c
		}
	}
	if raw.SalaryCurrency != "" {
		cur = strings.ToUpper(raw.SalaryCurrency)
	}
	if cur == "" && (smin != nil || smax != nil) {
		cur = "USD"
	}
	if !currencyAllowed(cur, n.cfg.Salary.Currencies) {
		smin, smax, cur = nil, nil, ""
	}

	return Job{
		Title:           title,
		Company:         strings.TrimSpace(raw.Company),
		Description:     desc,
		Location:        location,
		URL:             u,
		PostedAt:        posted,
		Source:          source,
		IsRemote:        isRemote,
		RemoteScope:     scope,
		SalaryMin:       smin,
		SalaryMax:       smax,
		SalaryCurrency:  cur,
		Seniority:       n.class.NormalizeSeniority(raw.NativeSeniority, title),
		RoleFamily:      roleFamily,
		NativeSeniority: raw.NativeSeniority,
	}, true
}

func postedTime(raw RawJob) *time.Time {
	if raw.PostedEpoch != nil {
		if t, ok := NormalizeEpoch(*raw.PostedEpoch); ok {
			return t
		}
	}
	if t, err := NormalizeTimestamp(raw.PostedAt); err == nil {
		return t
	}
	return nil
}

func currencyAllowed(cur string, allowed []string) bool {
	if cur == "" {
		return true
	}
	if len(allowed) == 0 {
		return true
	}
	for _, c := range allowed {
		if strings.EqualFold(c, cur) {
			return true
		}
	}
	return false
}

// canonicalizeURL strips fragments and tracking parameters.
func canonicalizeURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if u.Host == "" {
		return "", &url.Error{Op: "canonicalize", URL: raw, Err: errHost}
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	q := u.Query()
	for k := range q {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "utm_") || lk == "gh_src" || lk == "ref" || lk == "lever-source" {
			q.Del(k)
		}
	}
	u.RawQuery = q.Encode()
	u.Path = strings.TrimRight(u.Path, "/")
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String(), nil
}

type urlErr string

func (e urlErr) Error() string { return string(e) }

var errHost = urlErr("missing host")

var _ = context.Background
