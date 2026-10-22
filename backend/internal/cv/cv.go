package cv

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ContentHash is the dedupe key for an uploaded CV.
func ContentHash(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

var (
	dateRangeRe = regexp.MustCompile(`(?i)\b(jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)[a-z]*\.?\s+((?:19|20)\d{2})\s*(?:-|–|to)\s*(?:(jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)[a-z]*\.?\s+)?((?:19|20)\d{2}|present|current)\b`)
	yearOnlyRe  = regexp.MustCompile(`\b((?:19|20)\d{2})\s*(?:-|–|to)\s*((?:19|20)\d{2}|present|current)\b`)
)

// YearsFromDateRanges estimates total years of experience by merging
// non-overlapping date ranges. This is the no-key baseline.
func YearsFromDateRanges(text string, now time.Time) (float64, error) {
	type span struct{ start, end time.Time }
	var spans []span
	seen := map[string]bool{}

	for _, m := range dateRangeRe.FindAllStringSubmatch(text, -1) {
		start, ok := monthYear(m[1], m[2])
		if !ok {
			continue
		}
		end, ok := monthYearOrNow(m[3], m[4], now)
		if !ok {
			continue
		}
		key := start.Format("2006-01") + end.Format("2006-01")
		if !seen[key] {
			seen[key] = true
			spans = append(spans, span{start, end})
		}
	}
	if len(spans) == 0 {
		for _, m := range yearOnlyRe.FindAllStringSubmatch(text, -1) {
			start, err1 := time.Parse("2006", m[1])
			if err1 != nil {
				continue
			}
			end, ok := yearOrNow(m[2], now)
			if !ok {
				continue
			}
			key := m[1] + m[2]
			if !seen[key] {
				seen[key] = true
				spans = append(spans, span{start, end})
			}
		}
	}
	if len(spans) == 0 {
		return 0, errors.New("no date ranges found")
	}

	sort.Slice(spans, func(i, j int) bool { return spans[i].start.Before(spans[j].start) })
	var months float64
	cur := spans[0]
	for _, s := range spans[1:] {
		if s.start.After(cur.end) {
			months += monthsBetween(cur.start, cur.end)
			cur = s
			continue
		}
		if s.end.After(cur.end) {
			cur.end = s.end
		}
	}
	months += monthsBetween(cur.start, cur.end)
	return round1(months / 12), nil
}

func monthsBetween(a, b time.Time) float64 {
	return float64(b.Year()-a.Year())*12 + float64(b.Month()-a.Month())
}

var monthIndex = map[string]time.Month{
	"jan": time.January, "feb": time.February, "mar": time.March, "apr": time.April,
	"may": time.May, "jun": time.June, "jul": time.July, "aug": time.August,
	"sep": time.September, "oct": time.October, "nov": time.November, "dec": time.December,
}

func monthYear(name, year string) (time.Time, bool) {
	mon, ok := monthIndex[strings.ToLower(name[:3])]
	if !ok {
		return time.Time{}, false
	}
	y, err := strconv.Atoi(year)
	if err != nil {
		return time.Time{}, false
	}
	return time.Date(y, mon, 1, 0, 0, 0, 0, time.UTC), true
}

func monthYearOrNow(name, year string, now time.Time) (time.Time, bool) {
	if year == "" {
		return time.Time{}, false
	}
	l := strings.ToLower(year)
	if l == "present" || l == "current" {
		return now, true
	}
	if name == "" {
		y, err := strconv.Atoi(year)
		if err != nil {
			return time.Time{}, false
		}
		return time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC), true
	}
	return monthYear(name, year)
}

func yearOrNow(year string, now time.Time) (time.Time, bool) {
	l := strings.ToLower(year)
	if l == "present" || l == "current" {
		return now, true
	}
	y, err := strconv.Atoi(year)
	if err != nil {
		return time.Time{}, false
	}
	return time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC), true
}

func round1(f float64) float64 {
	return float64(int(f*10+0.5)) / 10
}
