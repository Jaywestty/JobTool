package ingest

import (
	"regexp"
	"strings"
)

// classifier holds compiled match patterns derived from the YAML config.
type classifier struct {
	cfg         *Config
	families    []compiledFamily
	blocked     []*regexp.Regexp
	scopeRegex  []*regexp.Regexp
	remoteTerms []*regexp.Regexp
	worldTerms  []*regexp.Regexp
	titleExcl   []*regexp.Regexp
	stopwords   map[string]bool
}

type compiledFamily struct {
	cfg    RoleFamily
	terms  []*regexp.Regexp
	alsoOK func(string) bool
}

func newClassifier(cfg *Config) *classifier {
	c := &classifier{
		cfg:       cfg,
		stopwords: defaultStopwords(),
	}
	for _, rf := range cfg.RoleFamilies {
		cf := compiledFamily{cfg: rf}
		for _, kw := range rf.Keywords {
			cf.terms = append(cf.terms, wholeWord(kw))
		}
		c.families = append(c.families, cf)
	}
	for _, p := range cfg.Language.BlockedPhrases {
		if re, err := regexp.Compile(`(?i)` + wholeWordPattern(p)); err == nil {
			c.blocked = append(c.blocked, re)
		}
	}
	for _, p := range cfg.RemoteScope.RestrictedPatterns {
		if re, err := regexp.Compile("(?i)" + p); err == nil {
			c.scopeRegex = append(c.scopeRegex, re)
		}
	}
	for _, t := range cfg.RemoteScope.RemoteTerms {
		c.remoteTerms = append(c.remoteTerms, wholeWord(t))
	}
	for _, t := range cfg.RemoteScope.WorldwideTerms {
		c.worldTerms = append(c.worldTerms, wholeWord(t))
	}
	for _, t := range cfg.Gate.TitleExcludeTerms {
		if re := wholeWord(t); re != nil {
			c.titleExcl = append(c.titleExcl, re)
		}
	}
	return c
}

// wholeWordPattern builds a case-insensitive whole-word regex source.
func wholeWordPattern(s string) string {
	esc := regexp.QuoteMeta(strings.TrimSpace(s))
	return `(?:^|[^a-z0-9])` + esc + `(?:[^a-z0-9]|$)`
}

func wholeWord(s string) *regexp.Regexp {
	re, err := regexp.Compile(`(?i)` + wholeWordPattern(s))
	if err != nil {
		return nil
	}
	return re
}

// classifyRoleFamily returns the first family whose keyword matches the title;
// only when the title has no match does it fall back to the description head.
// A match in the description head alone is weaker, so a title hit always wins.
func (c *classifier) classifyRoleFamily(title, description string) string {
	if !c.cfg.Gate.Enabled {
		return primaryFamily(c.cfg)
	}
	if fam := c.matchFamily(title); fam != "" {
		return fam
	}
	if c.titleExcluded(title) {
		return ""
	}
	return c.matchFamily(descriptionHead(description, c.cfg.Gate.DescriptionHeadChars))
}

// titleExcluded reports whether a title names a non-engineering role, which
// suppresses description-only classification.
func (c *classifier) titleExcluded(title string) bool {
	for _, re := range c.titleExcl {
		if re != nil && re.MatchString(title) {
			return true
		}
	}
	return false
}

func (c *classifier) matchFamily(text string) string {
	for _, f := range c.families {
		for _, re := range f.terms {
			if re != nil && re.MatchString(text) {
				return f.cfg.Slug
			}
		}
	}
	return ""
}

func primaryFamily(cfg *Config) string {
	if len(cfg.RoleFamilies) == 0 {
		return ""
	}
	return cfg.RoleFamilies[0].Slug
}

func descriptionHead(desc string, n int) string {
	if n <= 0 || len(desc) <= n {
		return desc
	}
	return desc[:n]
}

// englishOK reports whether the posting is in English.
func (c *classifier) englishOK(title, description string) (bool, string) {
	lc := c.cfg.Language
	if !lc.Enabled {
		return true, ""
	}
	text := title + "\n" + descriptionHead(description, lc.HeadChars)
	for _, re := range c.blocked {
		if re.MatchString(text) {
			return false, "blocked-phrase"
		}
	}
	if lc.MinEnglishRatio <= 0 {
		return true, ""
	}
	ratio, n := englishRatio(text, c.stopwords)
	// Very short postings carry too little signal; the blocked-phrase check
	// above already handled explicit requirements.
	if n < minEnglishTokens {
		return true, ""
	}
	if ratio < lc.MinEnglishRatio {
		return false, "non-english"
	}
	return true, ""
}

// minEnglishTokens is the shortest description the ratio check applies to.
const minEnglishTokens = 12

var tokenRe = regexp.MustCompile(`[a-z']+`)

func englishRatio(text string, stop map[string]bool) (float64, int) {
	toks := tokenRe.FindAllString(strings.ToLower(text), -1)
	if len(toks) == 0 {
		return 0, 0
	}
	hits := 0
	for _, t := range toks {
		if stop[t] {
			hits++
		}
	}
	return float64(hits) / float64(len(toks)), len(toks)
}

func defaultStopwords() map[string]bool {
	words := strings.Fields(`the be to of and a in that have it for not on with as you do at this
	but by from they we or an will all would there their what so up out if about who get which we
	our your work team role you use with can build using experience strong able help across
	join looking are is be new well how more other than then into make take good some any these
	us because years we've we're what who how when where why also its our has had was were been`)
	m := map[string]bool{}
	for _, w := range words {
		m[w] = true
	}
	return m
}
