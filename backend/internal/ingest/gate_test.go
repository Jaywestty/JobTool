package ingest

import "testing"

func testConfig() *Config {
	return &Config{
		RoleFamilies: []RoleFamily{
			{Slug: "backend", Keywords: []string{"backend", "golang", "api engineer"}},
			{Slug: "ai_ml", Keywords: []string{"machine learning", "llm", "data scientist"}},
			{Slug: "data", Keywords: []string{"data engineer"}},
		},
		Gate: GateConfig{Enabled: true, DescriptionHeadChars: 360, TitleExcludeTerms: []string{"customer success", "product manager", "marketing"}},
		Language: LangConfig{
			Enabled:         true,
			MinEnglishRatio: 0.12,
			HeadChars:       1500,
			BlockedPhrases:  []string{"german required", "fluent german"},
		},
		RemoteScope: ScopeConfig{
			RemoteTerms:    []string{"remote", "work from home"},
			WorldwideTerms: []string{"worldwide", "anywhere", "global", "emea"},
			RestrictedPatterns: []string{
				`\b(us|usa|united states)\s+only\b`,
				`\bmust be based in\b`,
			},
		},
		Seniority: Seniority{Levels: []string{"junior", "mid", "senior", "staff", "principal"}},
	}
}

func TestClassifyRoleFamily(t *testing.T) {
	c := newClassifier(testConfig())
	longDesc := "We are looking for someone. " + repeat("filler ", 60) + "data engineer"

	tests := []struct {
		name  string
		title string
		desc  string
		want  string
	}{
		{name: "title match", title: "Senior Backend Engineer", desc: "", want: "backend"},
		{name: "whole word only", title: "Fulfillment Coordinator", desc: "", want: ""},
		{name: "description head fallback", title: "Software Engineer", desc: "You will build LLM systems.", want: "ai_ml"},
		{name: "keyword beyond head ignored", title: "Engineer", desc: longDesc, want: ""},
		{name: "unrelated", title: "Sales Manager", desc: "Drive revenue.", want: ""},
		{name: "non-tech title blocks description match", title: "Customer Success Manager", desc: "Work with our machine learning customers.", want: ""},
		{name: "title match beats exclusion list", title: "Marketing Data Engineer", desc: "", want: "data"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := c.classifyRoleFamily(tt.title, tt.desc); got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestEnglishGate(t *testing.T) {
	c := newClassifier(testConfig())
	tests := []struct {
		name  string
		title string
		desc  string
		ok    bool
	}{
		{name: "english", title: "Backend Engineer", desc: "You will build and maintain services with the team. You will work with Go and the team to ship features.", ok: true},
		{name: "blocked phrase", title: "Backend Engineer", desc: "German required for this role. You build services.", ok: false},
		{name: "non-english", title: "Entwickler", desc: "Wir suchen einen Entwickler fuer unser Team. Sie arbeiten mit uns an neuen Projekten und Systemen in Berlin. Bewerben Sie sich jetzt bei uns mit Ihren Unterlagen.", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, reason := c.englishOK(tt.title, tt.desc)
			if ok != tt.ok {
				t.Fatalf("ok = %v (%s) want %v", ok, reason, tt.ok)
			}
		})
	}
}

func TestRemoteScope(t *testing.T) {
	c := newClassifier(testConfig())
	tests := []struct {
		name     string
		title    string
		loc      string
		desc     string
		isRemote bool
		want     string
	}{
		{name: "onsite none", title: "Engineer", loc: "Berlin", isRemote: false, want: ""},
		{name: "worldwide", title: "Backend Engineer", loc: "Remote", desc: "Work from anywhere in the world.", isRemote: true, want: ScopeWorldwide},
		{name: "restricted us only", title: "Engineer", loc: "Remote", desc: "US only position.", isRemote: true, want: ScopeRestricted},
		{name: "restricted must be based", title: "Engineer", loc: "Remote", desc: "You must be based in Canada.", isRemote: true, want: ScopeRestricted},
		{name: "restricted wins over worldwide", title: "Engineer", loc: "Remote worldwide", desc: "US only.", isRemote: true, want: ScopeRestricted},
		{name: "unknown", title: "Engineer", loc: "Remote", desc: "Build the platform with us.", isRemote: true, want: ScopeUnknown},
		{name: "cet is not restricted", title: "Engineer", loc: "Remote", desc: "You should overlap with CET time zones.", isRemote: true, want: ScopeUnknown},
		{name: "uk time is not restricted", title: "Engineer", loc: "Remote", desc: "Core hours are UK time.", isRemote: true, want: ScopeUnknown},
		{name: "remote detected from location", title: "Engineer", loc: "Remote - Europe", desc: "Global team.", isRemote: false, want: ScopeWorldwide},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := c.ClassifyRemoteScope(tt.title, tt.loc, tt.desc, tt.isRemote); got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeSeniority(t *testing.T) {
	c := newClassifier(testConfig())
	tests := []struct {
		native string
		title  string
		want   string
	}{
		{native: "Senior", title: "Engineer", want: "senior"},
		{native: "", title: "Junior Backend Engineer", want: "junior"},
		{native: "", title: "Staff Platform Engineer", want: "staff"},
		{native: "", title: "Backend Engineer", want: "unknown"},
	}
	for _, tt := range tests {
		if got := c.NormalizeSeniority(tt.native, tt.title); got != tt.want {
			t.Fatalf("NormalizeSeniority(%q,%q) = %q want %q", tt.native, tt.title, got, tt.want)
		}
	}
}

func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}
