package render

import (
	"fmt"
	"math"
	"time"
)

// tick is one labeled position on the axis header.
type tick struct {
	col   int
	label string
}

const maxTicks = 8

var absoluteSteps = []time.Duration{
	time.Hour,
	3 * time.Hour,
	6 * time.Hour,
	12 * time.Hour,
	24 * time.Hour,
	2 * 24 * time.Hour,
	7 * 24 * time.Hour,
	14 * 24 * time.Hour,
	30 * 24 * time.Hour,
	90 * 24 * time.Hour,
	180 * 24 * time.Hour,
	365 * 24 * time.Hour,
	2 * 365 * 24 * time.Hour,
	5 * 365 * 24 * time.Hour,
	10 * 365 * 24 * time.Hour,
}

var symbolicSteps = []int{1, 2, 5, 10, 20, 50, 100, 200, 500, 1000, 2000, 5000, 10000}

// buildTicks picks a "nice" step for the timeline's axis and returns
// tick marks within [sc.min, sc.max], collapsing any that would land
// on the same column.
func buildTicks(tl Timeline, sc scale) []tick {
	var raw []tick
	if tl.Axis == AxisSymbolic {
		raw = symbolicTicks(sc)
	} else {
		raw = absoluteTicks(sc)
	}

	var out []tick
	lastCol := -1
	for _, t := range raw {
		if t.col == lastCol {
			continue
		}
		out = append(out, t)
		lastCol = t.col
	}
	return out
}

func absoluteTicks(sc scale) []tick {
	span := sc.max - sc.min
	step := absoluteSteps[len(absoluteSteps)-1]
	for _, s := range absoluteSteps {
		step = s
		if span/s.Seconds() <= maxTicks {
			break
		}
	}

	format := "Jan 02"
	switch {
	case step < 24*time.Hour:
		format = "15:04"
	case step >= 365*24*time.Hour:
		format = "2006"
	}

	start := time.Unix(int64(sc.min), 0).UTC().Truncate(step)

	var ticks []tick
	for t := start; float64(t.Unix()) <= sc.max; t = t.Add(step) {
		if float64(t.Unix()) < sc.min {
			continue
		}
		ticks = append(ticks, tick{col: sc.col(float64(t.Unix())), label: t.Format(format)})
	}
	return ticks
}

func symbolicTicks(sc scale) []tick {
	span := sc.max - sc.min
	step := symbolicSteps[len(symbolicSteps)-1]
	for _, s := range symbolicSteps {
		step = s
		if span/float64(s) <= maxTicks {
			break
		}
	}

	start := int(math.Floor(sc.min/float64(step))) * step

	var ticks []tick
	for v := start; float64(v) <= sc.max; v += step {
		if float64(v) < sc.min {
			continue
		}
		ticks = append(ticks, tick{col: sc.col(float64(v)), label: fmt.Sprintf("T%d", v)})
	}
	return ticks
}
