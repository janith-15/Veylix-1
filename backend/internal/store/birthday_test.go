package store

import (
	"testing"
	"time"
)

// Age must come from the birth date, because a stored age is wrong the day after
// it is written and every year thereafter.
func TestAgeFromBirthday(t *testing.T) {
	cases := []struct {
		name     string
		birthday string
		now      string
		want     int
	}{
		{"birthday already passed this year", "1990-03-15", "2026-10-30", 36},
		{"birthday later this year", "1990-12-25", "2026-10-30", 35},
		{"on the birthday itself", "1990-10-30", "2026-10-30", 36},
		{"day before the birthday", "1990-10-31", "2026-10-30", 35},
		// The leap-year trap: YearDay comparison would make this 35, not 36.
		{"leap-day birthday in a non-leap year", "1988-02-29", "2026-03-01", 38},
		{"leap-day birthday on the day before", "1988-02-29", "2026-02-28", 37},
		{"unset is unknown, not zero", "", "2026-10-30", -1},
		{"unparseable is unknown", "30/10/1990", "2026-10-30", -1},
		{"future date is refused", "2030-01-01", "2026-10-30", -1},
	}
	for _, c := range cases {
		now, err := time.Parse("2006-01-02", c.now)
		if err != nil {
			t.Fatalf("%s: bad test date: %v", c.name, err)
		}
		if got := AgeFromBirthday(c.birthday, now); got != c.want {
			t.Errorf("%s: AgeFromBirthday(%q, %s) = %d, want %d",
				c.name, c.birthday, c.now, got, c.want)
		}
	}
}
