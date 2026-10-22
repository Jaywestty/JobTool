package ingest

import (
	"regexp"
	"strconv"
	"strings"
)

// Currency tokens recognised in free text.
var currencySymbols = map[string]string{
	"$": "USD",
	"€": "EUR",
	"£": "GBP",
	"₦": "NGN",
}

var (
	// A range needs a currency token on at least one side, so years, ratings
	// and counts ("2019", "5 - 10") are never read as salaries.
	salaryRangeRe  = regexp.MustCompile(`(?i)(\$|€|£|₦|\b(?:usd|eur|gbp|ngn|cad|aud)\b)\s*([0-9][0-9,\.]*)\s*([km])?\s*(?:-|–|—|to)\s*(?:\$|€|£|₦|\b(?:usd|eur|gbp|ngn|cad|aud)\b)?\s*([0-9][0-9,\.]*)\s*([km])?`)
	salarySingleRe = regexp.MustCompile(`(?i)(\$|€|£|₦|\b(?:usd|eur|gbp|ngn|cad|aud)\b)\s*([0-9][0-9,\.]{3,})\s*([km])?`)
	yearlyRe       = regexp.MustCompile(`(?i)per\s+(year|annum)|/\s*(yr|year)|annual|a\s+year`)
	monthlyRe      = regexp.MustCompile(`(?i)per\s+(month|mo)\b|/\s*mo(nth)?\b|monthly`)
)

const (
	// maxSalary rejects parse artifacts (phone numbers, IDs).
	maxSalary = 100_000_000
	// minAnnualSalary rejects values that cannot be a full-time annual salary
	// in the supported currencies, which removes stray small numbers.
	minAnnualSalary = 1000
)

// ParseSalary extracts an annual salary range from numeric fields, or from a
// free-text string that names a currency.
func ParseSalary(text string, minV, maxV *float64, allowed []string) (min, max *int64, currency string) {
	if minV != nil || maxV != nil {
		mn := toInt64Ptr(minV)
		mx := toInt64Ptr(maxV)
		if mn == nil && mx == nil {
			return nil, nil, ""
		}
		if mn == nil {
			mn = mx
		}
		if mx == nil {
			mx = mn
		}
		return mn, mx, ""
	}
	t := strings.TrimSpace(text)
	if t == "" {
		return nil, nil, ""
	}
	allowedSet := map[string]bool{}
	for _, c := range allowed {
		allowedSet[strings.ToUpper(c)] = true
	}
	pick := func(cur string) string {
		if cur == "" {
			return ""
		}
		if len(allowedSet) == 0 || allowedSet[cur] {
			return cur
		}
		return ""
	}
	normCur := func(tok string) string {
		if c, ok := currencySymbols[tok]; ok {
			return c
		}
		return strings.ToUpper(strings.TrimSpace(tok))
	}
	annualise := func(v int64) int64 {
		if !yearlyRe.MatchString(t) && monthlyRe.MatchString(t) {
			v *= 12
		}
		return v
	}

	if m := salaryRangeRe.FindStringSubmatch(t); m != nil {
		lo, okLo := parseMoney(m[2], m[3])
		hi, okHi := parseMoney(m[4], m[5])
		cur := pick(normCur(m[1]))
		if cur == "" {
			return nil, nil, ""
		}
		if !okLo && !okHi {
			return nil, nil, ""
		}
		if okLo {
			lo = clampSalary(annualise(lo))
		}
		if okHi {
			hi = clampSalary(annualise(hi))
		}
		if lo <= 0 {
			lo = hi
		}
		if hi <= 0 {
			hi = lo
		}
		return ptrIfPositive(lo), ptrIfPositive(hi), cur
	}
	if m := salarySingleRe.FindStringSubmatch(t); m != nil {
		v, ok := parseMoney(m[2], m[3])
		cur := pick(normCur(m[1]))
		if !ok || cur == "" {
			return nil, nil, ""
		}
		v = clampSalary(annualise(v))
		return ptrIfPositive(v), ptrIfPositive(v), cur
	}
	return nil, nil, ""
}

// clampSalary applies the plausible-range rules, returning 0 to drop a value.
func clampSalary(v int64) int64 {
	if v < minAnnualSalary || v > maxSalary {
		return 0
	}
	return v
}

func ptrIfPositive(v int64) *int64 {
	if v <= 0 {
		return nil
	}
	return &v
}

func parseMoney(num, suffix string) (int64, bool) {
	s := strings.ReplaceAll(strings.ReplaceAll(num, ",", ""), " ", "")
	if s == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f <= 0 {
		return 0, false
	}
	switch strings.ToLower(suffix) {
	case "k":
		f *= 1000
	case "m":
		f *= 1_000_000
	}
	if f > maxSalary {
		return 0, false
	}
	return int64(f), true
}

func toInt64Ptr(f *float64) *int64 {
	if f == nil || *f <= 0 || *f > maxSalary {
		return nil
	}
	v := int64(*f)
	return &v
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
