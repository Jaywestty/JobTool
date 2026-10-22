package cv

import (
	"testing"
	"time"
)

func TestYearsFromDateRanges(t *testing.T) {
	now := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		text string
		want float64
		ok   bool
	}{
		{
			name: "single range",
			text: "Software Engineer, Jan 2020 - Jan 2023",
			want: 3,
			ok:   true,
		},
		{
			name: "overlapping ranges are merged, not summed",
			text: "Jan 2020 - Jan 2023 at A\nFeb 2022 - Feb 2024 at B",
			want: 4.1,
			ok:   true,
		},
		{
			name: "non-overlapping ranges sum",
			text: "Jan 2015 - Jan 2017 at A\nJan 2020 - Jan 2023 at B",
			want: 5,
			ok:   true,
		},
		{
			name: "present counts to now",
			text: "Mar 2024 - Present",
			want: 2.6,
			ok:   true,
		},
		{
			name: "year pairs fallback",
			text: "2018 - 2020",
			want: 2,
			ok:   true,
		},
		{
			name: "no ranges",
			text: "Experienced engineer with strong Go skills.",
			want: 0,
			ok:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := YearsFromDateRanges(tt.text, now)
			if tt.ok && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.ok && err == nil {
				t.Fatal("expected an error")
			}
			if got != tt.want {
				t.Fatalf("got %.2f want %.2f", got, tt.want)
			}
		})
	}
}

func TestContentHashStable(t *testing.T) {
	a := ContentHash([]byte("same bytes"))
	b := ContentHash([]byte("same bytes"))
	c := ContentHash([]byte("other bytes"))
	if a != b {
		t.Fatal("hash is not stable")
	}
	if a == c {
		t.Fatal("different inputs produced the same hash")
	}
}
