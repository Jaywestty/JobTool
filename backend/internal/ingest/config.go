package ingest

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the whole ingestion configuration, loaded from YAML.
type Config struct {
	RoleFamilies []RoleFamily  `yaml:"role_families"`
	Gate         GateConfig    `yaml:"gate"`
	Language     LangConfig    `yaml:"language"`
	RemoteScope  ScopeConfig   `yaml:"remote_scope"`
	Sources      SourcesConfig `yaml:"sources"`
	Salary       SalaryConfig  `yaml:"salary"`
	Seniority    Seniority     `yaml:"seniority"`
}

type RoleFamily struct {
	Slug        string   `yaml:"slug"`
	Keywords    []string `yaml:"keywords"`
	AlsoMatches []string `yaml:"also_matches"`
}

type GateConfig struct {
	Enabled              bool     `yaml:"enabled"`
	DescriptionHeadChars int      `yaml:"description_head_chars"`
	TitleExcludeTerms    []string `yaml:"title_exclude_terms"`
}

type LangConfig struct {
	Enabled         bool     `yaml:"enabled"`
	MinEnglishRatio float64  `yaml:"min_english_ratio"`
	HeadChars       int      `yaml:"head_chars"`
	BlockedPhrases  []string `yaml:"blocked_phrases"`
}

type ScopeConfig struct {
	RemoteTerms        []string `yaml:"remote_terms"`
	WorldwideTerms     []string `yaml:"worldwide_terms"`
	RestrictedPatterns []string `yaml:"restricted_patterns"`
}

type SalaryConfig struct {
	Currencies []string `yaml:"currencies"`
}

type Seniority struct {
	Levels []string `yaml:"levels"`
}

type SourcesConfig struct {
	ATS            ATSConfig      `yaml:"ats"`
	RemoteOK       SourceOpts     `yaml:"remoteok"`
	Remotive       RemotiveOpts   `yaml:"remotive"`
	Himalayas      HimalayasOpts  `yaml:"himalayas"`
	Jobicy         JobicyOpts     `yaml:"jobicy"`
	WeWorkRemotely SourceOpts     `yaml:"weworkremotely"`
	Arbeitnow      ArbeitnowOpts  `yaml:"arbeitnow"`
	WorkingNomads  SourceOpts     `yaml:"workingnomads"`
	PythonOrg      SourceOpts     `yaml:"pythonorg"`
	HackerNews     HackerNewsOpts `yaml:"hackernews"`
}

type ATSConfig struct {
	Greenhouse ATSSlugs `yaml:"greenhouse"`
	Lever      ATSSlugs `yaml:"lever"`
	Ashby      ATSSlugs `yaml:"ashby"`
}

type ATSSlugs struct {
	Enabled         bool     `yaml:"enabled"`
	IntervalSeconds int      `yaml:"interval_seconds"`
	Slugs           []string `yaml:"slugs"`
}

type SourceOpts struct {
	Enabled         bool `yaml:"enabled"`
	IntervalSeconds int  `yaml:"interval_seconds"`
}

type RemotiveOpts struct {
	SourceOpts `yaml:",inline"`
	Searches   []string `yaml:"searches"`
}

type HimalayasOpts struct {
	SourceOpts `yaml:",inline"`
	MaxPages   int `yaml:"max_pages"`
}

type JobicyOpts struct {
	SourceOpts `yaml:",inline"`
	Count      int `yaml:"count"`
}

type ArbeitnowOpts struct {
	SourceOpts `yaml:",inline"`
	MaxPages   int `yaml:"max_pages"`
}

type HackerNewsOpts struct {
	SourceOpts `yaml:",inline"`
	MonthsBack int `yaml:"months_back"`
}

// LoadConfig reads and validates a YAML file. Validation covers everything a
// connector reads at runtime so a bad file fails at startup.
func LoadConfig(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read ingest config: %w", err)
	}
	var cfg Config
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(false)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse ingest config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// DefaultConfigPath is used when JOBTOOL_INGEST_CONFIG is unset.
const DefaultConfigPath = "config/ingest.yaml"

// Validate rejects configurations that would misbehave at run time.
func (c *Config) Validate() error {
	var errs []string
	add := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }

	if len(c.RoleFamilies) == 0 {
		add("role_families must not be empty")
	}
	slugs := map[string]bool{}
	for i, rf := range c.RoleFamilies {
		if strings.TrimSpace(rf.Slug) == "" {
			add("role_families[%d] has an empty slug", i)
		} else if slugs[rf.Slug] {
			add("duplicate role family %q", rf.Slug)
		}
		slugs[rf.Slug] = true
		if len(rf.Keywords) == 0 {
			add("role family %q has no keywords", rf.Slug)
		}
	}
	for _, rf := range c.RoleFamilies {
		for _, other := range rf.AlsoMatches {
			if !slugs[other] {
				add("role family %q also_matches unknown family %q", rf.Slug, other)
			}
		}
	}
	if c.Gate.Enabled && c.Gate.DescriptionHeadChars <= 0 {
		add("gate.description_head_chars must be positive when the gate is enabled")
	}
	if c.Language.Enabled {
		if c.Language.MinEnglishRatio < 0 || c.Language.MinEnglishRatio > 1 {
			add("language.min_english_ratio must be between 0 and 1")
		}
		if c.Language.HeadChars <= 0 {
			add("language.head_chars must be positive when the language gate is enabled")
		}
	}
	for i, pat := range c.RemoteScope.RestrictedPatterns {
		if _, err := regexp.Compile(pat); err != nil {
			add("remote_scope.restricted_patterns[%d] is not a valid regex: %v", i, err)
		}
	}
	if len(c.RemoteScope.RemoteTerms) == 0 {
		add("remote_scope.remote_terms must not be empty")
	}
	validateSource := func(name string, enabled bool, interval int) {
		if enabled && interval <= 0 {
			add("sources.%s.interval_seconds must be positive", name)
		}
	}
	for name, ats := range map[string]ATSSlugs{"greenhouse": c.Sources.ATS.Greenhouse, "lever": c.Sources.ATS.Lever, "ashby": c.Sources.ATS.Ashby} {
		validateSource("ats."+name, ats.Enabled, ats.IntervalSeconds)
	}
	validateSource("remoteok", c.Sources.RemoteOK.Enabled, c.Sources.RemoteOK.IntervalSeconds)
	validateSource("remotive", c.Sources.Remotive.Enabled, c.Sources.Remotive.IntervalSeconds)
	validateSource("himalayas", c.Sources.Himalayas.Enabled, c.Sources.Himalayas.IntervalSeconds)
	validateSource("jobicy", c.Sources.Jobicy.Enabled, c.Sources.Jobicy.IntervalSeconds)
	validateSource("weworkremotely", c.Sources.WeWorkRemotely.Enabled, c.Sources.WeWorkRemotely.IntervalSeconds)
	validateSource("arbeitnow", c.Sources.Arbeitnow.Enabled, c.Sources.Arbeitnow.IntervalSeconds)
	validateSource("workingnomads", c.Sources.WorkingNomads.Enabled, c.Sources.WorkingNomads.IntervalSeconds)
	validateSource("pythonorg", c.Sources.PythonOrg.Enabled, c.Sources.PythonOrg.IntervalSeconds)
	validateSource("hackernews", c.Sources.HackerNews.Enabled, c.Sources.HackerNews.IntervalSeconds)

	if c.Sources.Himalayas.Enabled && c.Sources.Himalayas.MaxPages <= 0 {
		add("sources.himalayas.max_pages must be positive")
	}
	if c.Sources.Arbeitnow.Enabled && c.Sources.Arbeitnow.MaxPages <= 0 {
		add("sources.arbeitnow.max_pages must be positive")
	}
	if c.Sources.Jobicy.Enabled && c.Sources.Jobicy.Count <= 0 {
		add("sources.jobicy.count must be positive")
	}
	if c.Sources.HackerNews.Enabled && c.Sources.HackerNews.MonthsBack <= 0 {
		add("sources.hackernews.months_back must be positive")
	}
	if len(errs) > 0 {
		return errors.New("invalid ingest config: " + strings.Join(errs, "; "))
	}
	return nil
}

// Interval converts a source's configured seconds to a Duration.
func Interval(seconds int) time.Duration { return time.Duration(seconds) * time.Second }
