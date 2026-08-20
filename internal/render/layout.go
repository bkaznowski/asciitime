package render

import (
	"math"
	"sort"
	"unicode/utf8"
)

// scale maps axis positions onto integer columns [0, width).
type scale struct {
	min, max float64
	width    int
}

func newScale(tl Timeline, width int) scale {
	min, max := math.Inf(1), math.Inf(-1)
	consider := func(p float64) {
		if p < min {
			min = p
		}
		if p > max {
			max = p
		}
	}
	for _, w := range tl.Windows {
		consider(w.Start.float())
		consider(w.End.float())
	}
	for _, e := range tl.Events {
		consider(e.Time.float())
		if e.Backdated {
			consider(e.RecordedAt.float())
		}
	}
	if math.IsInf(min, 1) {
		min, max = 0, 1
	}
	if min == max {
		max = min + 1
	}
	return scale{min: min, max: max, width: width}
}

func (s scale) col(p float64) int {
	frac := (p - s.min) / (s.max - s.min)
	c := int(math.Round(frac * float64(s.width-1)))
	if c < 0 {
		c = 0
	}
	if c > s.width-1 {
		c = s.width - 1
	}
	return c
}

// packWindowLanes assigns each window to the first lane it fits in
// without colliding with any window already there, processing windows
// in the order they were declared — so lane order (top to bottom)
// follows the YAML rather than being resorted by start time. This can
// occasionally use one more lane than the theoretical minimum (the
// classic sorted-greedy algorithm always finds the minimum), which is
// the accepted trade-off for keeping declaration order legible.
func packWindowLanes(windows []Window, sc scale) [][]Window {
	var lanes [][]Window
	for _, w := range windows {
		reserveStart, endCol := windowReservedRange(w, sc)
		placed := false
		for i := range lanes {
			if windowFitsLane(lanes[i], reserveStart, endCol, sc) {
				lanes[i] = append(lanes[i], w)
				placed = true
				break
			}
		}
		if !placed {
			lanes = append(lanes, []Window{w})
		}
	}
	return lanes
}

// windowReservedRange is the column span a window actually occupies,
// including its ID label drawn just left of the opening bracket.
func windowReservedRange(w Window, sc scale) (reserveStart, endCol int) {
	startCol := sc.col(w.Start.float())
	endCol = sc.col(w.End.float())
	reserveStart = startCol - utf8.RuneCountInString(w.ID)
	return reserveStart, endCol
}

// windowFitsLane reports whether a window with the given reserved
// range can share lane without its rendered columns overlapping any
// window already placed there.
func windowFitsLane(lane []Window, reserveStart, endCol int, sc scale) bool {
	for _, w := range lane {
		existingStart, existingEnd := windowReservedRange(w, sc)
		overlaps := endCol >= existingStart && existingEnd >= reserveStart
		if overlaps {
			return false
		}
	}
	return true
}

// eventMarker is one drawable item on the events section: either a
// single point event, several point events merged onto one column, or
// a backdated event spanning its effective and recorded columns.
//
// col is meaningful only for point/merged markers. effCol and recCol
// are meaningful only when isBackdated() — effCol is always where the
// x (effective time) is drawn and recCol is always where the >o/<o
// (recorded time) is drawn, regardless of which one comes first on the
// axis, so the arrow always points from effective to recorded.
// renderStart/renderEnd are the actual leftmost/rightmost columns this
// marker will draw into, used for lane packing so a marker whose label
// text runs left of its anchor column (the reversed-arrow case) still
// reserves the space it needs.
type eventMarker struct {
	events      []Event
	col         int
	effCol      int
	recCol      int
	renderStart int
	renderEnd   int
}

// eventGroup is one named block of events that renders as its own
// dedicated set of lanes, never sharing a lane or a merged clash
// marker with another group.
type eventGroup struct {
	name   string
	events []Event
}

// groupEvents partitions events by their Group field. Named groups
// keep the order they first appear in; ungrouped events (Group == "")
// always form the last block, with no header, regardless of where
// they first appeared, so mixing grouped and ungrouped events doesn't
// interleave them.
func groupEvents(events []Event) []eventGroup {
	var order []string
	byName := map[string][]Event{}
	seen := map[string]bool{}
	for _, e := range events {
		if !seen[e.Group] {
			seen[e.Group] = true
			order = append(order, e.Group)
		}
		byName[e.Group] = append(byName[e.Group], e)
	}

	var groups []eventGroup
	for _, name := range order {
		if name == "" {
			continue
		}
		groups = append(groups, eventGroup{name: name, events: byName[name]})
	}
	if ungrouped, ok := byName[""]; ok {
		groups = append(groups, eventGroup{events: ungrouped})
	}
	return groups
}

// buildEventMarkers turns events into markers. Point events (no
// RecordedAt) on the same column are merged into one marker unless
// StackClashingEvents is set. Backdated events are never merged since
// they occupy a range, not a point; overlapping backdated ranges are
// separated by lane packing instead, same as windows.
func buildEventMarkers(events []Event, sc scale, opts Options) []eventMarker {
	var points, backdated []Event
	for _, e := range events {
		if e.Backdated {
			backdated = append(backdated, e)
		} else {
			points = append(points, e)
		}
	}

	var markers []eventMarker

	newPointMarker := func(evs []Event, col int) eventMarker {
		m := eventMarker{events: evs, col: col}
		w := 1 + utf8.RuneCountInString(m.idList()) // "o" anchor + comma-joined IDs
		m.renderStart, m.renderEnd = col, col+w-1
		return m
	}

	if opts.StackClashingEvents {
		for _, e := range points {
			markers = append(markers, newPointMarker([]Event{e}, sc.col(e.Time.float())))
		}
	} else {
		byCol := map[int][]Event{}
		var cols []int
		for _, e := range points {
			c := sc.col(e.Time.float())
			if _, ok := byCol[c]; !ok {
				cols = append(cols, c)
			}
			byCol[c] = append(byCol[c], e)
		}
		sort.Ints(cols)
		for _, c := range cols {
			markers = append(markers, newPointMarker(byCol[c], c))
		}
	}

	for _, e := range backdated {
		effCol, recCol := sc.col(e.Time.float()), sc.col(e.RecordedAt.float())
		idLen := utf8.RuneCountInString(e.ID)
		m := eventMarker{events: []Event{e}, effCol: effCol, recCol: recCol}
		if effCol <= recCol {
			// x at effCol; "o"+ID spans recCol..recCol+idLen.
			m.renderStart, m.renderEnd = effCol, recCol+idLen
		} else {
			// ID+"o" spans recCol-idLen..recCol; x at effCol.
			m.renderStart, m.renderEnd = recCol-idLen, effCol
		}
		markers = append(markers, m)
	}

	return markers
}

// packEventLanes lays out markers with the same greedy algorithm as
// packWindowLanes, using each marker's actual rendered span so labels
// that grow leftward (a reversed backdated arrow) still reserve the
// room they need instead of only reserving space to the right.
func packEventLanes(markers []eventMarker) [][]eventMarker {
	sorted := append([]eventMarker(nil), markers...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].renderStart < sorted[j].renderStart })

	var lanes [][]eventMarker
	var laneEnd []int
	for _, m := range sorted {
		placed := false
		for i := range lanes {
			if laneEnd[i] < m.renderStart {
				lanes[i] = append(lanes[i], m)
				laneEnd[i] = m.renderEnd
				placed = true
				break
			}
		}
		if !placed {
			lanes = append(lanes, []eventMarker{m})
			laneEnd = append(laneEnd, m.renderEnd)
		}
	}
	return lanes
}
