package digest

import (
	"testing"
	"time"
)

func TestDue(t *testing.T) {
	loc, err := time.LoadLocation("Africa/Lagos")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	// 08:00 in Lagos equals 07:00 UTC.
	at08 := time.Date(2026, time.October, 12, 8, 0, 0, 0, loc)
	tests := []struct {
		name string
		pref Preference
		now  time.Time
		want bool
	}{
		{
			name: "never sent, right hour",
			pref: Preference{Frequency: Daily, Timezone: "Africa/Lagos", SendHour: 8, Enabled: true},
			now:  at08,
			want: true,
		},
		{
			name: "wrong hour",
			pref: Preference{Frequency: Daily, Timezone: "Africa/Lagos", SendHour: 8, Enabled: true},
			now:  at08.Add(time.Hour),
			want: false,
		},
		{
			name: "disabled",
			pref: Preference{Frequency: Daily, Timezone: "Africa/Lagos", SendHour: 8, Enabled: false},
			now:  at08,
			want: false,
		},
		{
			name: "already sent today",
			pref: Preference{Frequency: Daily, Timezone: "Africa/Lagos", SendHour: 8, Enabled: true, LastSent: ptr(at08.Add(-time.Minute))},
			now:  at08,
			want: false,
		},
		{
			name: "weekly too soon",
			pref: Preference{Frequency: Weekly, Timezone: "Africa/Lagos", SendHour: 8, Enabled: true, LastSent: ptr(at08.AddDate(0, 0, -3))},
			now:  at08,
			want: false,
		},
		{
			name: "weekly due",
			pref: Preference{Frequency: Weekly, Timezone: "Africa/Lagos", SendHour: 8, Enabled: true, LastSent: ptr(at08.AddDate(0, 0, -7))},
			now:  at08,
			want: true,
		},
		{
			name: "every 3 days due",
			pref: Preference{Frequency: Every3Day, Timezone: "Africa/Lagos", SendHour: 8, Enabled: true, LastSent: ptr(at08.AddDate(0, 0, -3))},
			now:  at08,
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Due(tt.pref, tt.now)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}

func TestDueRejectsBadInput(t *testing.T) {
	if _, err := Due(Preference{Frequency: "hourly", Timezone: "UTC", SendHour: 8, Enabled: true}, time.Now()); err == nil {
		t.Fatal("expected invalid frequency error")
	}
	if _, err := Due(Preference{Frequency: Daily, Timezone: "Not/AZone", SendHour: 8, Enabled: true}, time.Now()); err == nil {
		t.Fatal("expected invalid timezone error")
	}
}

func ptr(t time.Time) *time.Time { return &t }
