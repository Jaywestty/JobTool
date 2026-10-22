package digest

import (
	"errors"
	"time"
)

// Frequency is one of the three supported digest cadences.
type Frequency string

const (
	Daily     Frequency = "daily"
	Every3Day Frequency = "every_3_days"
	Weekly    Frequency = "weekly"
)

// Days returns the interval in days for a frequency.
func (f Frequency) Days() int {
	switch f {
	case Daily:
		return 1
	case Every3Day:
		return 3
	case Weekly:
		return 7
	default:
		return 0
	}
}

func (f Frequency) Valid() bool { return f.Days() > 0 }

// Preference is a user's digest settings.
type Preference struct {
	Frequency Frequency
	Timezone  string
	SendHour  int
	Channels  []string
	Enabled   bool
	LastSent  *time.Time
}

// Due reports whether a digest should be sent at now in the user's timezone.
func Due(p Preference, now time.Time) (bool, error) {
	if !p.Enabled {
		return false, nil
	}
	if !p.Frequency.Valid() {
		return false, errors.New("invalid digest frequency")
	}
	if p.SendHour < 0 || p.SendHour > 23 {
		return false, errors.New("invalid send hour")
	}
	loc, err := time.LoadLocation(p.Timezone)
	if err != nil {
		return false, err
	}
	local := now.In(loc)
	if local.Hour() != p.SendHour {
		return false, nil
	}
	if p.LastSent == nil {
		return true, nil
	}
	last := p.LastSent.In(loc)
	if last.YearDay() == local.YearDay() && last.Year() == local.Year() {
		return false, nil
	}
	interval := time.Duration(p.Frequency.Days()) * 24 * time.Hour
	return local.Sub(last) >= interval, nil
}
