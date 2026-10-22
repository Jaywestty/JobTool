package ingest

import (
	"strconv"
	"strings"
	"time"
)

// TimeValue is a JSON timestamp that may arrive as a number, a numeric string,
// ISO-8601, or RFC-822. Decode is not strict: it keeps the raw text when the
// number cannot be parsed so NormalizeTimestamp can still try string formats.
type TimeValue struct {
	Raw   string
	IsNum bool
	Num   float64
}

func (v TimeValue) Present() bool { return strings.TrimSpace(v.Raw) != "" || v.IsNum }

// String reports the raw textual form.
func (v TimeValue) String() string { return v.Raw }

// NormalizeTimestamp converts the supported encodings to UTC.
func NormalizeTimestamp(raw string) (*time.Time, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, nil
	}
	if n, err := strconv.ParseFloat(s, 64); err == nil {
		return fromEpoch(n)
	}
	if t, ok := parseISO(s); ok {
		return &t, nil
	}
	if t, ok := parseRFC822(s); ok {
		return &t, nil
	}
	for _, layout := range []string{"2006-01-02", "01/02/2006", "2006-01-02 15:04:05", "02 Jan 2006"} {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return &t, nil
		}
	}
	return nil, nil
}

// NormalizeEpoch converts epoch seconds or milliseconds to UTC.
func NormalizeEpoch(n float64) (*time.Time, bool) {
	if n <= 0 {
		return nil, false
	}
	if n >= 1e12 {
		n /= 1000
	}
	t := time.Unix(0, int64(n*1e9)).UTC()
	return &t, true
}

func fromEpoch(n float64) (*time.Time, error) {
	t, ok := NormalizeEpoch(n)
	if !ok {
		return nil, nil
	}
	return t, nil
}

func parseISO(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

func parseRFC822(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}
