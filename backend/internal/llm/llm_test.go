package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDictionaryBaseline(t *testing.T) {
	d := NewDictionary(map[string][]string{
		"go":       {"golang"},
		"postgres": {"postgresql"},
	})
	tests := []struct {
		name string
		text string
		want []string
	}{
		{name: "alias match", text: "Built services in Golang and PostgreSQL.", want: []string{"go", "postgres"}},
		{name: "no false positive on google", text: "Worked at Google.", want: nil},
		{name: "case insensitive", text: "GO and POSTGRES", want: []string{"go", "postgres"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := d.Extract(context.Background(), "cv", tt.text)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Skills) != len(tt.want) {
				t.Fatalf("got %v want %v", got.Skills, tt.want)
			}
			set := map[string]bool{}
			for _, s := range got.Skills {
				set[s] = true
			}
			for _, w := range tt.want {
				if !set[w] {
					t.Fatalf("missing %q in %v", w, got.Skills)
				}
			}
		})
	}
}

func TestOpenAICompatibleExtract(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Here you go:\n` + "```json" + `\n{\"skills\":[\"go\"],\"years\":4,\"seniority\":\"senior\",\"role_family\":\"backend\"}\n` + "```" + `"}}]}`))
	}))
	defer srv.Close()

	p := NewOpenAICompatible(srv.URL, "test-key", "test-model")
	got, err := p.Extract(context.Background(), "cv", "text")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(got.Skills) != 1 || got.Skills[0] != "go" {
		t.Fatalf("skills = %v", got.Skills)
	}
	if got.Years != 4 || got.RoleFamily != "backend" {
		t.Fatalf("unexpected extraction: %+v", got)
	}
}

func TestOpenAICompatibleUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("no"))
	}))
	defer srv.Close()

	p := NewOpenAICompatible(srv.URL, "", "m")
	if _, err := p.Extract(context.Background(), "cv", "x"); err == nil {
		t.Fatal("expected an error on 401")
	}
}
