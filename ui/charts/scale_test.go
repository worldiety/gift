package charts

import (
	"slices"
	"testing"
	"time"
)

func labels(ts []tick) []string {
	var ls []string
	for _, t := range ts {
		ls = append(ls, t.label)
	}
	return ls
}

func TestNumberTicksAreNiceAndCompact(t *testing.T) {
	if got := labels(numberTicks(0, 100, 300, 60, nil)); !slices.Equal(got, []string{"0", "20", "40", "60", "80", "100"}) {
		t.Errorf("0 to 100: %q", got)
	}
	if got := labels(numberTicks(-0.3, 0.3, 300, 60, nil)); !slices.Equal(got, []string{"-0.2", "0", "0.2"}) {
		t.Errorf("-0.3 to 0.3: %q", got)
	}
	if got := labels(numberTicks(0, 25000, 300, 60, nil)); !slices.Equal(got, []string{"0", "5k", "10k", "15k", "20k", "25k"}) {
		t.Errorf("0 to 25000: %q", got)
	}
	if lo, hi := niceDomain(4.3, 23.2, 5); lo != 0 || hi != 25 {
		t.Errorf("the domain of 4.3 to 23.2 is [%v, %v]", lo, hi)
	}
	if lo, hi := niceDomain(7, 7, 5); !(lo < 7 && hi > 7) {
		t.Errorf("the domain of a single value is [%v, %v]", lo, hi)
	}
}

func TestTimeTicksFollowTheCalendar(t *testing.T) {
	zone := time.FixedZone("CET", 3600)
	at := func(y int, m time.Month, d, h int) float64 { return Seconds(time.Date(y, m, d, h, 0, 0, 0, zone)) }

	day := labels(timeTicks(at(2026, 10, 30, 0), at(2026, 10, 31, 0), 600, 72, zone, nil))
	if !slices.Equal(day, []string{"30 Oct", "03:00", "06:00", "09:00", "12:00", "15:00", "18:00", "21:00", "31 Oct"}) {
		t.Errorf("a day: %q", day)
	}
	morning := labels(timeTicks(at(2026, 10, 30, 2), at(2026, 10, 30, 5), 600, 72, zone, nil))
	if len(morning) == 0 || morning[0] != "30 Oct, 02:00" {
		t.Errorf("a morning without its midnight: %q", morning)
	}
	year := labels(timeTicks(at(2026, 1, 1, 0), at(2027, 1, 1, 0), 600, 72, zone, nil))
	if year[0] != "2026" || !slices.Contains(year, "Jul") || year[len(year)-1] != "2027" {
		t.Errorf("a year: %q", year)
	}
	weeks := timeTicks(at(2026, 10, 1, 0), at(2026, 12, 1, 0), 400, 72, zone, nil)
	for _, w := range weeks {
		if d := Time(w.v).In(zone); d.Weekday() != time.Monday && d.Day() != 1 {
			t.Errorf("a tick on %v", d)
		}
	}
}

func TestInspectedTimesSayAsMuchAsTheyKnow(t *testing.T) {
	zone := time.FixedZone("CET", 3600)
	for _, c := range []struct {
		t    time.Time
		span float64
		want string
	}{
		{time.Date(2026, 10, 30, 13, 40, 0, 0, zone), 86400, "30 Oct 2026, 13:40"},
		{time.Date(2026, 10, 30, 13, 40, 12, 0, zone), 86400, "30 Oct 2026, 13:40:12"},
		{time.Date(2026, 10, 30, 13, 40, 12, 250e6, zone), 60, "30 Oct 2026, 13:40:12.250"},
		{time.Date(2026, 10, 30, 0, 0, 0, 0, zone), 365 * 86400, "30 Oct 2026"},
	} {
		if got := timeValue(c.t, c.span); got != c.want {
			t.Errorf("%v: %q, want %q", c.t, got, c.want)
		}
	}
}
