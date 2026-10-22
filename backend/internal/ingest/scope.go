package ingest

import "strings"

// RemoteScope values.
const (
	ScopeWorldwide  = "worldwide"
	ScopeRestricted = "restricted"
	ScopeUnknown    = "unknown"
)

// IsRemotePosting reports whether the posting claims to be remote.
func (c *classifier) IsRemotePosting(title, location string, isRemote bool) bool {
	if isRemote {
		return true
	}
	text := title + " " + location
	for _, re := range c.remoteTerms {
		if re != nil && re.MatchString(text) {
			return true
		}
	}
	return false
}

// ClassifyRemoteScope returns worldwide, restricted or unknown for remote
// postings, and empty for onsite postings. A scope is only assigned to a
// posting that is actually remote; CET and UK timezone requirements are
// intentionally not restricted terms.
func (c *classifier) ClassifyRemoteScope(title, location, description string, isRemote bool) string {
	if !c.IsRemotePosting(title, location, isRemote) {
		return ""
	}
	text := title + "\n" + location + "\n" + description
	for _, re := range c.scopeRegex {
		if re.MatchString(text) {
			return ScopeRestricted
		}
	}
	for _, re := range c.worldTerms {
		if re != nil && re.MatchString(text) {
			return ScopeWorldwide
		}
	}
	return ScopeUnknown
}

// NormalizeSeniority maps source-native labels and title cues to a config
// level. Native labels win.
func (c *classifier) NormalizeSeniority(native, title string) string {
	levels := c.cfg.Seniority.Levels
	lowerLevels := make([]string, len(levels))
	for i, l := range levels {
		lowerLevels[i] = strings.ToLower(l)
	}
	if native != "" {
		n := strings.ToLower(native)
		for i, l := range lowerLevels {
			if strings.Contains(n, l) {
				return levels[i]
			}
		}
	}
	t := strings.ToLower(title)
	for i := len(lowerLevels) - 1; i >= 0; i-- {
		if wholeWord(lowerLevels[i]).MatchString(t) {
			return levels[i]
		}
	}
	return "unknown"
}
