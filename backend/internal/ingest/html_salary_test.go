package ingest

import "testing"

func TestStripHTML(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "tags", in: "<p>Hello <b>world</b></p>", want: "Hello world"},
		{name: "entities", in: "R&amp;D &lt;team&gt;", want: "R&D <team>"},
		{name: "comments", in: "A<!-- hidden -->B", want: "A B"},
		{name: "collapse", in: "a \n\n  b\tc", want: "a b c"},
		{name: "nbsp", in: "a\u00a0b", want: "a b"},
		{name: "empty", in: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StripHTML(tt.in); got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestFirstLine(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "Remote, USA<p>Body</p>", want: "Remote, USA"},
		{in: "Berlin\nBody", want: "Berlin"},
		{in: "  only text  ", want: "only text"},
	}
	for _, tt := range tests {
		if got := FirstLine(tt.in); got != tt.want {
			t.Fatalf("FirstLine(%q) = %q want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseSalary(t *testing.T) {
	allowed := []string{"USD", "EUR", "GBP"}
	tests := []struct {
		name    string
		text    string
		wantMin int64
		wantMax int64
		wantCur string
	}{
		{name: "dollar range", text: "Salary: $120,000 - $160,000 per year", wantMin: 120000, wantMax: 160000, wantCur: "USD"},
		{name: "k suffix", text: "EUR 80k-100k", wantMin: 80000, wantMax: 100000, wantCur: "EUR"},
		{name: "single", text: "GBP 75000 per annum", wantMin: 75000, wantMax: 75000, wantCur: "GBP"},
		{name: "monthly annualised", text: "USD 8,000 per month", wantMin: 96000, wantMax: 96000, wantCur: "USD"},
		{name: "no salary", text: "Competitive", wantMin: 0, wantMax: 0, wantCur: ""},
		{name: "bare year is not salary", text: "Founded in 2019 - 2024", wantMin: 0, wantMax: 0, wantCur: ""},
		{name: "bare small range is not salary", text: "5 - 10 years experience", wantMin: 0, wantMax: 0, wantCur: ""},
		{name: "rating is not salary", text: "Rated 4.5 out of 5", wantMin: 0, wantMax: 0, wantCur: ""},
		{name: "disallowed currency", text: "NGN 5,000,000 - 8,000,000", wantMin: 0, wantMax: 0, wantCur: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mn, mx, cur := ParseSalary(tt.text, nil, nil, allowed)
			if tt.wantMin == 0 {
				if mn != nil || mx != nil {
					t.Fatalf("expected nil salary, got %v-%v", mn, mx)
				}
				return
			}
			if mn == nil || mx == nil || *mn != tt.wantMin || *mx != tt.wantMax || cur != tt.wantCur {
				t.Fatalf("got %v-%v %q want %d-%d %q", deref(mn), deref(mx), cur, tt.wantMin, tt.wantMax, tt.wantCur)
			}
		})
	}
}

func deref(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}
