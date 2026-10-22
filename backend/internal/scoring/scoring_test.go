package scoring

import (
	"testing"

	"github.com/jobtool/jobtool/backend/internal/platform"
)

func testConfig() Config {
	return Config{
		Weights: Weights{RequiredSkills: 0.35, Seniority: 0.20, RoleFamily: 0.15, Embedding: 0.20, Recency: 0.10},
		Gates:   Gates{RecencyMaxDays: 60, MaxSeniorityGap: 2, MaxYearsGap: 4, RequireRoleFamily: true},
	}
}

func testFamilies() platform.RoleFamilies {
	return platform.RoleFamilies{
		Slugs:    []string{"backend", "ai_ml"},
		Keywords: map[string][]string{"backend": {"go"}, "ai_ml": {"pytorch"}},
	}
}

func TestScoreIsDeterministicAndExplainable(t *testing.T) {
	cfg := testConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("config should be valid: %v", err)
	}
	p := Profile{
		Skills:          map[string]bool{"go": true, "postgres": true, "docker": true},
		YearsExperience: 5,
		SeniorityRank:   2,
		RoleFamily:      "backend",
		Embedding:       []float64{1, 0, 0},
	}
	j := Candidate{
		RequiredSkills: []string{"go", "postgres", "kubernetes", "docker"},
		SeniorityRank:  2,
		YearsRequired:  4,
		RoleFamily:     "backend",
		Embedding:      []float64{1, 0, 0},
		AgeDays:        10,
		RemoteScope:    "remote",
	}

	first := Score(cfg, p, j)
	second := Score(cfg, p, j)
	if first.Score != second.Score {
		t.Fatalf("score is not deterministic: %v then %v", first.Score, second.Score)
	}
	if first.Score <= 0 || first.Score > 100 {
		t.Fatalf("score out of range: %v", first.Score)
	}
	if len(first.Breakdown) != 5 {
		t.Fatalf("expected 5 components, got %d", len(first.Breakdown))
	}
	if len(first.MatchedSkills) != 3 || len(first.MissingSkills) != 1 {
		t.Fatalf("matched=%v missing=%v", first.MatchedSkills, first.MissingSkills)
	}
	if first.MissingSkills[0] != "kubernetes" {
		t.Fatalf("expected kubernetes missing, got %v", first.MissingSkills)
	}
	var sum float64
	for _, c := range first.Breakdown {
		sum += c.Points
	}
	if diff := sum - first.Score; diff > 0.05 || diff < -0.05 {
		t.Fatalf("breakdown points %v do not add to score %v", sum, first.Score)
	}
}

func TestGates(t *testing.T) {
	cfg := testConfig()
	families := testFamilies()
	base := Profile{Skills: map[string]bool{"go": true}, YearsExperience: 5, SeniorityRank: 2, RoleFamily: "backend"}
	baseJob := Candidate{RequiredSkills: []string{"go"}, SeniorityRank: 2, RoleFamily: "backend", AgeDays: 5, RemoteScope: "remote"}

	tests := []struct {
		name    string
		mutate  func(*Candidate)
		passed  bool
	}{
		{name: "passes", mutate: func(*Candidate) {}, passed: true},
		{name: "missing role family", mutate: func(c *Candidate) { c.RoleFamily = "" }, passed: false},
		{name: "out of scope family", mutate: func(c *Candidate) { c.RoleFamily = "sales" }, passed: false},
		{name: "seniority gap", mutate: func(c *Candidate) { c.SeniorityRank = 5 }, passed: false},
		{name: "years gap", mutate: func(c *Candidate) { c.YearsRequired = 12 }, passed: false},
		{name: "too old", mutate: func(c *Candidate) { c.AgeDays = 120 }, passed: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := baseJob
			tt.mutate(&job)
			got := ApplyGates(cfg, families, base, job)
			if got.Passed != tt.passed {
				t.Fatalf("passed=%v want %v (reason %q)", got.Passed, tt.passed, got.Reason)
			}
		})
	}
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name   string
		a, b   []float64
		want   float64
	}{
		{name: "identical", a: []float64{1, 0}, b: []float64{1, 0}, want: 1},
		{name: "orthogonal", a: []float64{1, 0}, b: []float64{0, 1}, want: 0},
		{name: "dimension mismatch is zero", a: []float64{1, 0}, b: []float64{1}, want: 0},
		{name: "empty is zero", a: nil, b: nil, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CosineSimilarity(tt.a, tt.b); got != tt.want {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}

func TestConfigRejectsBadWeights(t *testing.T) {
	cfg := testConfig()
	cfg.Weights.Recency = 0.5
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected weights that do not sum to 1.0 to be rejected")
	}
}
