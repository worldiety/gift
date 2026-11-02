package showcase

import (
	"math"
	"time"
)

// chartZone is the time zone of the demos, so that their axes read the same
// on every machine.
var chartZone = time.FixedZone("CET", 3600)

// chartDay is the day the demos measure.
var chartDay = time.Date(2026, 10, 30, 0, 0, 0, 0, chartZone)

// noise is a repeatable wobble in [-1, 1].
func noise(i int) float64 {
	return math.Sin(float64(i)*12.9898+math.Sin(float64(i)*0.37)*4) * math.Cos(float64(i)*4.1414)
}
