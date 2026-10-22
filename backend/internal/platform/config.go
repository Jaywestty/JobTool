package platform

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config is the fully validated runtime configuration.
type Config struct {
	AppEnv       string
	Role         string
	HTTPAddr     string
	DatabaseURL  string
	LogLevel     string
	Migrations   bool
	WebOrigin    string
	RoleFamilies RoleFamilies
	Embedding    Embedding
	LLM          LLM
	Digest       Digest
	Sources      Sources
}

type Embedding struct {
	Provider string
	Model    string
	Dim      int
}

type LLM struct {
	Provider string
	BaseURL  string
	APIKey   string
	Model    string
	TimeoutS int
}

type Digest struct {
	DefaultFrequency string
	SendHour         int
}

type Sources struct {
	UserAgent string
	TimeoutS  int
}

// RoleFamilies maps a role family slug to its keywords, in config order.
type RoleFamilies struct {
	Slugs    []string
	Keywords map[string][]string
}

func (r RoleFamilies) Valid(slug string) bool {
	_, ok := r.Keywords[slug]
	return ok
}

var validRoles = map[string]bool{"api": true, "worker": true, "scheduler": true}

var validFrequencies = map[string]bool{"daily": true, "every_3_days": true, "weekly": true}

// Load reads configuration from the environment. Nothing defaults silently:
// every value is either present or rejected. roleOverride, when non-empty,
// takes precedence over JOBTOOL_ROLE (the --role flag).
func Load(getenv func(string) string) (*Config, error) {
	return load(getenv, "")
}

// LoadWithRole is Load with the command-line role applied over the environment.
func LoadWithRole(getenv func(string) string, roleOverride string) (*Config, error) {
	return load(getenv, roleOverride)
}

func load(getenv func(string) string, roleOverride string) (*Config, error) {
	var errs []string
	get := func(key string) string { return strings.TrimSpace(getenv(key)) }
	required := func(key string) string {
		v := get(key)
		if v == "" {
			errs = append(errs, key+" is required")
		}
		return v
	}
	intIn := func(key string, min, max int) int {
		raw := required(key)
		if raw == "" {
			return 0
		}
		n, err := strconv.Atoi(raw)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s must be an integer, got %q", key, raw))
			return 0
		}
		if n < min || n > max {
			errs = append(errs, fmt.Sprintf("%s must be between %d and %d, got %d", key, min, max, n))
		}
		return n
	}
	enum := func(key, v string, allowed map[string]bool) string {
		if v != "" && !allowed[v] {
			keys := make([]string, 0, len(allowed))
			for k := range allowed {
				keys = append(keys, k)
			}
			errs = append(errs, fmt.Sprintf("%s must be one of %s, got %q", key, strings.Join(keys, ", "), v))
		}
		return v
	}

	roleEnv := get("JOBTOOL_ROLE")
	if roleOverride != "" {
		roleEnv = roleOverride
	}
	// The role may be absent only for a migration-only run; main enforces a
	// valid role before starting any long-lived process.
	role := enum("JOBTOOL_ROLE", roleEnv, validRoles)
	migrateRaw := get("JOBTOOL_MIGRATE")
	migrate := false
	if migrateRaw != "" {
		b, err := strconv.ParseBool(migrateRaw)
		if err != nil {
			errs = append(errs, fmt.Sprintf("JOBTOOL_MIGRATE must be a boolean, got %q", migrateRaw))
		}
		migrate = b
	}

	families, err := parseRoleFamilies(required("JOBTOOL_ROLE_FAMILIES"))
	if err != nil {
		errs = append(errs, "JOBTOOL_ROLE_FAMILIES: "+err.Error())
	}

	cfg := &Config{
		AppEnv:      get("JOBTOOL_APP_ENV"),
		Role:        role,
		HTTPAddr:    required("JOBTOOL_HTTP_ADDR"),
		DatabaseURL: required("JOBTOOL_DATABASE_URL"),
		LogLevel:    enum("JOBTOOL_LOG_LEVEL", required("JOBTOOL_LOG_LEVEL"), map[string]bool{"debug": true, "info": true, "warn": true, "error": true}),
		Migrations:  migrate,
		WebOrigin:   required("JOBTOOL_WEB_ORIGIN"),
		RoleFamilies: families,
		Embedding: Embedding{
			Provider: enum("JOBTOOL_EMBEDDING_PROVIDER", required("JOBTOOL_EMBEDDING_PROVIDER"), map[string]bool{"local": true, "openai_compatible": true}),
			Model:    required("JOBTOOL_EMBEDDING_MODEL"),
			Dim:      intIn("JOBTOOL_EMBEDDING_DIM", 8, 8192),
		},
		LLM: LLM{
			Provider: enum("JOBTOOL_LLM_PROVIDER", required("JOBTOOL_LLM_PROVIDER"), map[string]bool{"none": true, "ollama": true, "openai_compatible": true}),
			BaseURL:  get("JOBTOOL_LLM_BASE_URL"),
			APIKey:   get("JOBTOOL_LLM_API_KEY"),
			Model:    get("JOBTOOL_LLM_MODEL"),
			TimeoutS: intIn("JOBTOOL_LLM_TIMEOUT_S", 1, 600),
		},
		Digest: Digest{
			DefaultFrequency: enum("JOBTOOL_DIGEST_DEFAULT_FREQUENCY", required("JOBTOOL_DIGEST_DEFAULT_FREQUENCY"), validFrequencies),
			SendHour:         intIn("JOBTOOL_DIGEST_SEND_HOUR", 0, 23),
		},
		Sources: Sources{
			UserAgent: get("JOBTOOL_SOURCES_USER_AGENT"),
			TimeoutS:  intIn("JOBTOOL_SOURCES_TIMEOUT_S", 1, 300),
		},
	}

	if cfg.LLM.Provider != "none" {
		if cfg.LLM.BaseURL == "" {
			errs = append(errs, "JOBTOOL_LLM_BASE_URL is required when JOBTOOL_LLM_PROVIDER is "+cfg.LLM.Provider)
		}
		if cfg.LLM.Model == "" {
			errs = append(errs, "JOBTOOL_LLM_MODEL is required when JOBTOOL_LLM_PROVIDER is "+cfg.LLM.Provider)
		}
	}

	if len(errs) > 0 {
		return nil, errors.New("invalid configuration: " + strings.Join(errs, "; "))
	}
	return cfg, nil
}

// LoadFromEnv is Load bound to the process environment.
func LoadFromEnv() (*Config, error) { return Load(os.Getenv) }

// parseRoleFamilies decodes a JSON array of {slug, keywords} objects.
func parseRoleFamilies(raw string) (RoleFamilies, error) {
	var entries []struct {
		Slug     string   `json:"slug"`
		Keywords []string `json:"keywords"`
	}
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return RoleFamilies{}, fmt.Errorf("must be a JSON array of {slug, keywords}: %w", err)
	}
	families := RoleFamilies{Keywords: map[string][]string{}}
	for i, e := range entries {
		if e.Slug == "" {
			return RoleFamilies{}, fmt.Errorf("entry %d has an empty slug", i)
		}
		if len(e.Keywords) == 0 {
			return RoleFamilies{}, fmt.Errorf("entry %q has no keywords", e.Slug)
		}
		if _, dup := families.Keywords[e.Slug]; dup {
			return RoleFamilies{}, fmt.Errorf("duplicate slug %q", e.Slug)
		}
		families.Slugs = append(families.Slugs, e.Slug)
		families.Keywords[e.Slug] = e.Keywords
	}
	if len(families.Slugs) == 0 {
		return RoleFamilies{}, errors.New("at least one role family is required")
	}
	return families, nil
}
