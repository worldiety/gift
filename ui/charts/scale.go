package charts

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
)

// tick is a value on an axis and its label.
type tick struct {
	v     float64
	label string
}

// niceStep returns the step of 1, 2 or 5 times a power of ten that divides
// span into about count parts, but never more.
func niceStep(span float64, count int) float64 {
	if !(span > 0) || math.IsInf(span, 0) || count < 1 {
		return 1
	}
	raw := span / float64(count)
	p := math.Pow(10, math.Floor(math.Log10(raw)))
	for _, m := range []float64{1, 2, 5, 10} {
		if m*p >= raw {
			return m * p
		}
	}
	return 10 * p
}

// numberTicks returns the multiples of a nice step in [lo, hi], about one
// for every spacing pixels of length px.
func numberTicks(lo, hi float64, px, spacing float32, format func(float64) string) []tick {
	count := max(int(px/spacing), 1)
	step := niceStep(hi-lo, count)
	f := format
	if f == nil {
		f = numberFormat(step)
	}
	var ts []tick
	first := math.Ceil(lo/step - 1e-9)
	for k := first; ; k++ {
		v := k * step
		if v > hi+step*1e-9 || len(ts) > 1000 {
			break
		}
		if v == 0 {
			v = 0 // no "-0"
		}
		ts = append(ts, tick{v, f(v)})
	}
	return ts
}

// niceDomain widens [lo, hi] to multiples of the step its ticks would have,
// as a chart's automatic domain does, and makes an empty one a range.
func niceDomain(lo, hi float64, count int) (float64, float64) {
	if lo == hi {
		d := math.Max(math.Abs(lo)*0.1, 1)
		lo, hi = lo-d, hi+d
	}
	step := niceStep(hi-lo, max(count, 1))
	return math.Floor(lo/step+1e-9) * step, math.Ceil(hi/step-1e-9) * step
}

// numberFormat returns a format with as many decimals as step needs, and
// thousands as k and millions as M from ten thousand on, as a compact axis
// label wants them.
func numberFormat(step float64) func(float64) string {
	decimals := 0
	if step > 0 && step < 1 {
		decimals = int(math.Ceil(-math.Log10(step) - 1e-9))
	}
	return func(v float64) string { return formatNumber(v, decimals, step) }
}

func formatNumber(v float64, decimals int, step float64) string {
	if v == 0 {
		return "0"
	}
	a := math.Abs(v)
	for _, s := range []struct {
		div    float64
		suffix string
	}{{1e9, "G"}, {1e6, "M"}, {1e3, "k"}} {
		if a >= 10*s.div || a >= s.div && step >= s.div/10 {
			d := 0
			if q := step / s.div; q < 1 {
				d = int(math.Ceil(-math.Log10(q) - 1e-9))
			}
			return trimZeros(strconv.FormatFloat(v/s.div, 'f', d, 64)) + s.suffix
		}
	}
	return strconv.FormatFloat(v, 'f', decimals, 64)
}

func trimZeros(s string) string {
	if !strings.Contains(s, ".") {
		return s
	}
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

// valueFormat is the format of a single value, as the inspection shows it:
// four significant digits, more when they are whole.
func valueFormat(v float64) string {
	if Missing(v) {
		return "–"
	}
	a := math.Abs(v)
	switch {
	case a == 0:
		return "0"
	case a >= 1e6 || a < 1e-3:
		return strconv.FormatFloat(v, 'g', 4, 64)
	}
	d := max(0, 3-int(math.Floor(math.Log10(a))))
	return trimZeros(strconv.FormatFloat(v, 'f', d, 64))
}

// timeStep is a step of a time axis: a duration below a day, or a number of
// days, months or years.
type timeStep struct {
	d     time.Duration
	days  int
	month int
	years int
}

// approx is the step in seconds, for choosing one.
func (s timeStep) approx() float64 {
	switch {
	case s.years > 0:
		return float64(s.years) * 365.25 * 86400
	case s.month > 0:
		return float64(s.month) * 30.44 * 86400
	case s.days > 0:
		return float64(s.days) * 86400
	}
	return s.d.Seconds()
}

var timeSteps = func() []timeStep {
	var ss []timeStep
	for _, d := range []time.Duration{
		time.Millisecond, 2 * time.Millisecond, 5 * time.Millisecond, 10 * time.Millisecond,
		20 * time.Millisecond, 50 * time.Millisecond, 100 * time.Millisecond, 200 * time.Millisecond,
		500 * time.Millisecond, time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second,
		15 * time.Second, 30 * time.Second, time.Minute, 2 * time.Minute, 5 * time.Minute,
		10 * time.Minute, 15 * time.Minute, 30 * time.Minute, time.Hour, 2 * time.Hour,
		3 * time.Hour, 6 * time.Hour, 12 * time.Hour,
	} {
		ss = append(ss, timeStep{d: d})
	}
	for _, n := range []int{1, 2, 7, 14} {
		ss = append(ss, timeStep{days: n})
	}
	for _, n := range []int{1, 2, 3, 6} {
		ss = append(ss, timeStep{month: n})
	}
	for _, n := range []int{1, 2, 5, 10, 20, 50, 100} {
		ss = append(ss, timeStep{years: n})
	}
	return ss
}()

// timeTicks returns the ticks of a time axis over [lo, hi], in seconds, at
// the finest step whose labels are at least spacing pixels apart: hours on
// the hour, days at midnight, months on the first, in loc.
func timeTicks(lo, hi float64, px, spacing float32, loc *time.Location, format func(float64) string) []tick {
	span := hi - lo
	if !(span > 0) || px <= 0 {
		return nil
	}
	step := timeSteps[len(timeSteps)-1]
	for _, s := range timeSteps {
		if float32(s.approx()/span)*px >= spacing {
			step = s
			break
		}
	}
	var ts []tick
	add := func(t time.Time) bool {
		x := Seconds(t)
		if x > hi || len(ts) > 1000 {
			return false
		}
		if x >= lo {
			l := ""
			if format != nil {
				l = format(x)
			} else {
				l = timeLabel(t, step)
			}
			ts = append(ts, tick{x, l})
		}
		return true
	}
	start := Time(lo).In(loc)
	y, m, d := start.Date()
	switch {
	case step.years > 0:
		for yy := y - y%step.years; add(time.Date(yy, 1, 1, 0, 0, 0, 0, loc)); yy += step.years {
		}
	case step.month > 0:
		mm := int(m) - 1
		mm -= mm % step.month
		for k := 0; add(time.Date(y, time.Month(mm+1+k*step.month), 1, 0, 0, 0, 0, loc)); k++ {
		}
	case step.days > 0:
		day := time.Date(y, m, d, 0, 0, 0, 0, loc)
		if step.days == 7 || step.days == 14 {
			// Weeks begin on Monday.
			day = day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
		}
		for k := 0; add(day.AddDate(0, 0, k*step.days)); k++ {
		}
	default:
		day := time.Date(y, m, d, 0, 0, 0, 0, loc)
		first := day.Add(start.Sub(day).Truncate(step.d))
		for t := first; add(t); t = t.Add(step.d) {
		}
		// Without a midnight in view the times would not say which day
		// they are of; the first one says it.
		if format == nil && len(ts) > 0 && !slices.ContainsFunc(ts, func(t tick) bool { return isMidnight(Time(t.v).In(loc)) }) {
			t := Time(ts[0].v).In(loc)
			ts[0].label = t.Format("2 Jan") + ", " + ts[0].label
		}
	}
	return ts
}

func isMidnight(t time.Time) bool {
	return t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 && t.Nanosecond() == 0
}

// timeLabel labels t on an axis of step: the time of day for steps below a
// day, with the date at midnight; the day for days, with the year on New
// Year's day; the month for months, with the year in January; the year.
func timeLabel(t time.Time, s timeStep) string {
	midnight := isMidnight(t)
	newYear := midnight && t.YearDay() == 1
	switch {
	case s.years > 0:
		return t.Format("2006")
	case s.month > 0:
		if t.Month() == time.January {
			return t.Format("2006")
		}
		return t.Format("Jan")
	case s.days > 0:
		if newYear {
			return t.Format("2006")
		}
		return t.Format("2 Jan")
	case s.d < time.Second:
		return t.Format("15:04:05.000")
	case s.d < time.Minute:
		return t.Format("15:04:05")
	case midnight:
		return t.Format("2 Jan")
	}
	return t.Format("15:04")
}

// timeValue is the label of a single time, as the inspection shows it: the
// date, and the time as far as it is not zero, unless the chart spans
// months.
func timeValue(t time.Time, span float64) string {
	switch {
	case t.Nanosecond() != 0:
		return t.Format("2 Jan 2006, 15:04:05.000")
	case t.Second() != 0:
		return t.Format("2 Jan 2006, 15:04:05")
	case !isMidnight(t) || span < 2*86400:
		return t.Format("2 Jan 2006, 15:04")
	}
	return t.Format("2 Jan 2006")
}
