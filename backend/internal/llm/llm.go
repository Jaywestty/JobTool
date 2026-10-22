package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jobtool/jobtool/backend/internal/platform"
)

// Extraction is what an extractor may return from a CV or job description.
type Extraction struct {
	Skills      []string `json:"skills"`
	Years       float64  `json:"years"`
	Seniority   string   `json:"seniority"`
	RoleFamily  string   `json:"role_family"`
	Location    string   `json:"location"`
	RemoteScope string   `json:"remote_scope"`
}

// Provider is the optional LLM boundary. Implementations never compute scores.
type Provider interface {
	Name() string
	Extract(ctx context.Context, kind string, text string) (Extraction, error)
}

// Dictionary is the no-key baseline: it maps text to canonical skills through
// the skills table. It is the default and must work end to end.
type Dictionary map[string]string // lowercased alias -> canonical slug

// NewDictionary builds a baseline extractor from alias rows.
func NewDictionary(rows map[string][]string) Dictionary {
	d := Dictionary{}
	for canonical, aliases := range rows {
		d[strings.ToLower(canonical)] = canonical
		for _, a := range aliases {
			d[strings.ToLower(strings.TrimSpace(a))] = canonical
		}
	}
	return d
}

func (d Dictionary) Name() string { return "dictionary" }

func (d Dictionary) Extract(ctx context.Context, kind, text string) (Extraction, error) {
	lower := strings.ToLower(text)
	found := map[string]bool{}
	for alias, canonical := range d {
		if containsWord(lower, alias) {
			found[canonical] = true
		}
	}
	skills := make([]string, 0, len(found))
	for s := range found {
		skills = append(skills, s)
	}
	return Extraction{Skills: skills}, nil
}

// containsWord does a bounded match so "go" does not match "google".
func containsWord(haystack, needle string) bool {
	if needle == "" {
		return false
	}
	idx := 0
	for {
		pos := strings.Index(haystack[idx:], needle)
		if pos < 0 {
			return false
		}
		start := idx + pos
		end := start + len(needle)
		leftOK := start == 0 || !isWordByte(haystack[start-1])
		rightOK := end == len(haystack) || !isWordByte(haystack[end])
		if leftOK && rightOK {
			return true
		}
		idx = start + 1
		if idx >= len(haystack) {
			return false
		}
	}
}

func isWordByte(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// FromConfig returns the configured provider, or the baseline when the
// configuration asks for none. A configured provider that fails to construct
// is an error only when it was explicitly requested.
func FromConfig(cfg *platform.Config, baseline Provider) (Provider, error) {
	switch cfg.LLM.Provider {
	case "", "none":
		return baseline, nil
	case "ollama", "openai_compatible":
		return NewOpenAICompatible(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.Model), nil
	default:
		return nil, errors.New("unsupported llm provider " + cfg.LLM.Provider)
	}
}

func parseError(status int, body string) error {
	return fmt.Errorf("llm provider returned status %d: %s", status, strings.TrimSpace(body))
}
