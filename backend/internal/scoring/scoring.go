package scoring

import (
	"errors"
	"math"
	"strings"

	"github.com/jobtool/jobtool/backend/internal/platform"
)

// Weights are the weighted components. They sum to 1.0 and come from config.
type Weights struct {
	RequiredSkills float64
	Seniority      float64
	RoleFamily     float64
	Embedding      float64
	Recency        float64
}

// Gates are the hard gates applied before scoring.
type Gates struct {
	MinScore            float64
	MaxSeniorityGap     int
	MaxYearsGap         float64
	RecencyMaxDays      int
	RequireRoleFamily   bool
	AllowedRemoteScopes []string
}

// Config is the full scoring configuration.
type Config struct {
	Weights Weights
	Gates   Gates
}

// Validate rejects configurations that would silently distort scores.
func (c Config) Validate() error {
	sum := c.Weights.RequiredSkills + c.Weights.Seniority + c.Weights.RoleFamily +
		c.Weights.Embedding + c.Weights.Recency
	if math.Abs(sum-1.0) > 1e-6 {
		return errors.New("scoring weights must sum to 1.0")
	}
	if c.Gates.RecencyMaxDays <= 0 {
		return errors.New("scoring recency gate must be positive")
	}
	return nil
}

// SeniorityRank maps a seniority level to an ordinal. It is config-driven.
type SeniorityRank map[string]int

// Profile is the user side of a comparison.
type Profile struct {
	Skills             map[string]bool
	YearsExperience    float64
	SeniorityRank      int
	RoleFamily         string
	Embedding          []float64
	PreferredScopes    []string
}

// Candidate is the job side of a comparison.
type Candidate struct {
	ID             string
	RequiredSkills []string
	MustHaveSkills []string
	SeniorityRank  int
	YearsRequired  float64
	RoleFamily     string
	Embedding      []float64
	AgeDays        int
	RemoteScope    string
}

// Component is one weighted part of the breakdown.
type Component struct {
	Name    string  `json:"name"`
	Weight  float64 `json:"weight"`
	Raw     float64 `json:"raw"`
	Points  float64 `json:"points"`
	Detail  string  `json:"detail,omitempty"`
}

// Result is the explainable scoring output.
type Result struct {
	Score          float64     `json:"score"`
	Breakdown      []Component `json:"breakdown"`
	MatchedSkills  []string    `json:"matched_skills"`
	MissingSkills  []string    `json:"missing_skills"`
	ScoringVersion string      `json:"scoring_version"`
}

// GateResult reports whether a candidate passes the hard gates.
type GateResult struct {
	Passed bool
	Reason string
}

// ApplyGates evaluates hard gates. This is deterministic and explainable.
func ApplyGates(cfg Config, families platform.RoleFamilies, p Profile, j Candidate) GateResult {
	if cfg.Gates.RequireRoleFamily && j.RoleFamily == "" {
		return GateResult{Reason: "job has no role family"}
	}
	if !families.Valid(j.RoleFamily) && j.RoleFamily != "" {
		return GateResult{Reason: "role family not in scope"}
	}
	if cfg.Gates.MaxSeniorityGap > 0 && absInt(j.SeniorityRank-p.SeniorityRank) > cfg.Gates.MaxSeniorityGap {
		return GateResult{Reason: "seniority gap too large"}
	}
	if cfg.Gates.MaxYearsGap > 0 && j.YearsRequired-p.YearsExperience > cfg.Gates.MaxYearsGap {
		return GateResult{Reason: "years required exceed experience"}
	}
	if cfg.Gates.RecencyMaxDays > 0 && j.AgeDays > cfg.Gates.RecencyMaxDays {
		return GateResult{Reason: "posting too old"}
	}
	if len(cfg.Gates.AllowedRemoteScopes) > 0 && !contains(cfg.Gates.AllowedRemoteScopes, j.RemoteScope) {
		return GateResult{Reason: "remote scope not allowed"}
	}
	return GateResult{Passed: true}
}

// Score computes a 0-100 score plus breakdown. The LLM never enters here.
func Score(cfg Config, p Profile, j Candidate) Result {
	matched, missing := skillDiff(p.Skills, j.RequiredSkills)

	skillRaw := 0.0
	if len(j.RequiredSkills) > 0 {
		skillRaw = float64(len(matched)) / float64(len(j.RequiredSkills))
	}

	seniorityRaw := seniorityFit(p, j)
	familyRaw := 0.0
	if j.RoleFamily != "" && j.RoleFamily == p.RoleFamily {
		familyRaw = 1.0
	}
	embeddingRaw := CosineSimilarity(p.Embedding, j.Embedding)
	recencyRaw := recencyFit(cfg.Gates.RecencyMaxDays, j.AgeDays)

	w := cfg.Weights
	components := []Component{
		{Name: "required_skill_coverage", Weight: w.RequiredSkills, Raw: skillRaw,
			Detail: ratioDetail(len(matched), len(j.RequiredSkills))},
		{Name: "seniority_years_fit", Weight: w.Seniority, Raw: seniorityRaw},
		{Name: "role_family", Weight: w.RoleFamily, Raw: familyRaw},
		{Name: "embedding_similarity", Weight: w.Embedding, Raw: embeddingRaw},
		{Name: "recency", Weight: w.Recency, Raw: recencyRaw},
	}
	total := 0.0
	for i := range components {
		components[i].Points = round2(components[i].Weight * components[i].Raw * 100)
		total += components[i].Points
	}
	return Result{
		Score:          round2(total),
		Breakdown:      components,
		MatchedSkills:  matched,
		MissingSkills:  missing,
		ScoringVersion: "v1",
	}
}

func seniorityFit(p Profile, j Candidate) float64 {
	gap := absInt(j.SeniorityRank - p.SeniorityRank)
	switch gap {
	case 0:
		return 1.0
	case 1:
		return 0.5
	default:
		return 0.0
	}
}

func recencyFit(maxDays, ageDays int) float64 {
	if ageDays <= 0 {
		return 1.0
	}
	if ageDays >= maxDays {
		return 0.0
	}
	return 1.0 - float64(ageDays)/float64(maxDays)
}

func skillDiff(have map[string]bool, required []string) (matched, missing []string) {
	for _, s := range required {
		if have[s] {
			matched = append(matched, s)
		} else {
			missing = append(missing, s)
		}
	}
	return matched, missing
}

// CosineSimilarity returns a value in [0,1] for comparable vectors and 0
// otherwise. Dimension mismatch is a configuration error and never compares.
func CosineSimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	sim := dot / (math.Sqrt(na) * math.Sqrt(nb))
	return math.Max(0, math.Min(1, sim))
}

func ratioDetail(a, b int) string {
	return strings.TrimSpace(
		strings.Join([]string{itoa(a), "of", itoa(b), "required"}, " "),
	)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func round2(f float64) float64 { return math.Round(f*100) / 100 }

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
