package render

import "time"

// Axis selects whether positions are wall-clock timestamps or plain
// integer steps (T0, T1, T2, ...).
type Axis string

const (
	AxisAbsolute Axis = "absolute"
	AxisSymbolic Axis = "symbolic"
)

// Options controls layout choices that don't change the underlying data.
type Options struct {
	// StackClashingEvents, when true, gives every point event its own
	// lane instead of merging events that land on the same column into
	// a single combined marker.
	StackClashingEvents bool
}

// Timeline is the fully parsed, validated representation of one diagram.
type Timeline struct {
	Title   string
	Axis    Axis
	Options Options
	Windows []Window
	Events  []Event
}

// pos is a single point on the axis, either a wall-clock time or a plain
// integer step. Exactly one of the two forms is meaningful, selected by
// isAbs and consistent across an entire Timeline.
type pos struct {
	abs   time.Time
	sym   int
	isAbs bool
}

func absPos(t time.Time) pos { return pos{abs: t, isAbs: true} }
func symPos(n int) pos       { return pos{sym: n} }

// float returns the position on a single numeric axis suitable for
// column mapping: Unix seconds for absolute time, the raw step for
// symbolic time.
func (p pos) float() float64 {
	if p.isAbs {
		return float64(p.abs.Unix())
	}
	return float64(p.sym)
}

// Window is a labeled span between two points on the axis.
type Window struct {
	ID    string
	Label string
	Start pos
	End   pos
}

// Event is a labeled point on the axis. A backdated event additionally
// carries the time it was recorded, which may differ from Time. Group
// clusters events that belong together onto their own dedicated block
// of lanes, never sharing a lane — or a merged clash marker — with a
// differently-grouped event. The zero value means ungrouped.
type Event struct {
	ID         string
	Label      string
	Time       pos
	Backdated  bool
	RecordedAt pos
	Group      string
}
