package ingest

import (
	"testing"
	"time"
)

func TestNormalizeTimestamp(t *testing.T) {
	epoch := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		in   string
		want *time.Time
	}{
		{name: "empty", in: "  ", want: nil},
		{name: "epoch seconds", in: "1790856000", want: ptr(epoch)},
		{name: "epoch millis", in: "1790856000000", want: ptr(epoch)},
		{name: "iso8601 z", in: "2026-10-01T12:00:00Z", want: ptr(epoch)},
		{name: "iso8601 offset", in: "2026-10-01T13:00:00+01:00", want: ptr(epoch)},
		{name: "rfc822", in: "Thu, 01 Oct 2026 12:00:00 +0000", want: ptr(epoch)},
		{name: "date only", in: "2026-10-01", want: ptr(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))},
		{name: "garbage", in: "not a date", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeTimestamp(tt.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.want == nil {
				if got != nil {
					t.Fatalf("got %v want nil", *got)
				}
				return
			}
			if got == nil || !got.Equal(*tt.want) {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizeEpoch(t *testing.T) {
	tests := []struct {
		name string
		in   float64
		ok   bool
	}{
		{name: "zero rejected", in: 0, ok: false},
		{name: "negative rejected", in: -5, ok: false},
		{name: "seconds", in: 1790856000, ok: true},
		{name: "millis", in: 1790856000000, ok: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, ok := NormalizeEpoch(tt.in); ok != tt.ok {
				t.Fatalf("ok = %v want %v", ok, tt.ok)
			}
		})
	}
}

func ptr(t time.Time) *time.Time { return &t }
