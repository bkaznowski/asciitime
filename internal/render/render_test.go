package render

import (
	"strings"
	"testing"
)

func TestLoadAndRenderAbsolute(t *testing.T) {
	tl, err := LoadFile("../../testdata/contracts.yaml")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	out := Render(tl, 78)

	for _, want := range []string{"Contracts", "A", "B", "E1", "E2", "Legend", "effective"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestLoadAndRenderSymbolic(t *testing.T) {
	tl, err := LoadFile("../../testdata/protocol.yaml")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	out := Render(tl, 60)

	for _, want := range []string{"T0", "Handshake", "E2", "E3"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestGroupEventsOrdering(t *testing.T) {
	events := []Event{
		{ID: "N1"}, // ungrouped, appears first
		{ID: "E1", Group: "account"},
		{ID: "P1", Group: "payment"},
		{ID: "E2", Group: "account"},
	}
	groups := groupEvents(events)
	if len(groups) != 3 {
		t.Fatalf("got %d groups, want 3", len(groups))
	}
	if groups[0].name != "account" || groups[1].name != "payment" {
		t.Fatalf("named groups should keep first-appearance order, got %q then %q", groups[0].name, groups[1].name)
	}
	if groups[2].name != "" {
		t.Fatalf("ungrouped block should be last regardless of file order, got name %q", groups[2].name)
	}
	if len(groups[0].events) != 2 || groups[0].events[0].ID != "E1" || groups[0].events[1].ID != "E2" {
		t.Fatalf("account group should contain E1 then E2, got %+v", groups[0].events)
	}
}

func TestLoadAndRenderGrouped(t *testing.T) {
	tl, err := LoadFile("../../testdata/grouped.yaml")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	out := Render(tl, 78)

	if !strings.Contains(out, "account") || !strings.Contains(out, "payment") {
		t.Errorf("output missing group headers:\n%s", out)
	}
	// A marker's ID sits next to its o, but which side depends on
	// whether there was room on the preferred side (e.g. events at the
	// very last tick fall back to ID before o) — check adjacency in
	// either order rather than assuming one.
	for _, id := range []string{"E1", "E2", "P1", "P2", "N1"} {
		if !strings.Contains(out, "o"+id) && !strings.Contains(out, id+"o") {
			t.Errorf("output missing a marker for %q (expected adjacent to o):\n%s", id, out)
		}
	}

	// E2 (account) and P2 (payment) both land on T5 — same column, but
	// different groups must never merge into one "oE2,P2" marker.
	if strings.Contains(out, "E2,P2") || strings.Contains(out, "P2,E2") {
		t.Errorf("events from different groups must not merge on a shared column:\n%s", out)
	}
}

func TestUngroupedRenderIsUnchanged(t *testing.T) {
	tl := Timeline{
		Axis: AxisSymbolic,
		Events: []Event{
			{ID: "E1", Time: symPos(1)},
			{ID: "E2", Time: symPos(2)},
		},
	}
	out := Render(tl, 40)
	if strings.Contains(out, "\n\n\n") {
		t.Errorf("ungrouped events should render as a single flat block with no group-separator blank lines:\n%q", out)
	}
}

func TestWindowAutoID(t *testing.T) {
	cases := map[int]string{0: "A", 25: "Z", 26: "AA", 27: "AB"}
	for i, want := range cases {
		if got := windowAutoID(i); got != want {
			t.Errorf("windowAutoID(%d) = %q, want %q", i, got, want)
		}
	}
}

func TestPackWindowLanesSeparatesOverlaps(t *testing.T) {
	tl := Timeline{
		Axis: AxisSymbolic,
		Windows: []Window{
			{ID: "A", Start: symPos(0), End: symPos(4)},
			{ID: "B", Start: symPos(2), End: symPos(6)}, // overlaps A
			{ID: "C", Start: symPos(5), End: symPos(8)}, // does not overlap A
		},
	}
	sc := newScale(tl, 40)
	lanes := packWindowLanes(tl.Windows, sc)
	if len(lanes) != 2 {
		t.Fatalf("got %d lanes, want 2", len(lanes))
	}
}

func TestWindowLanesFollowDeclarationOrder(t *testing.T) {
	// Same overlap relationships regardless of declaration order, but
	// declared as W2, W1, W3. Under the old start-time-sorted packer,
	// W1 (the widest span) would always land on lane 0 no matter how
	// it was declared. Declaring W2 first must now put W2 on lane 0
	// instead.
	w2 := Window{ID: "W2", Start: symPos(10), End: symPos(20)}
	w1 := Window{ID: "W1", Start: symPos(0), End: symPos(100)}
	w3 := Window{ID: "W3", Start: symPos(30), End: symPos(40)}
	windows := []Window{w2, w1, w3}

	tl := Timeline{Axis: AxisSymbolic, Windows: windows}
	sc := newScale(tl, 120)
	lanes := packWindowLanes(windows, sc)

	if len(lanes) != 2 {
		t.Fatalf("got %d lanes, want 2: %+v", len(lanes), lanes)
	}
	if len(lanes[0]) != 2 || lanes[0][0].ID != "W2" || lanes[0][1].ID != "W3" {
		t.Fatalf("lane 0 should be [W2, W3] (declared-first W2, then non-overlapping W3), got %+v", lanes[0])
	}
	if len(lanes[1]) != 1 || lanes[1][0].ID != "W1" {
		t.Fatalf("lane 1 should be [W1], pushed off lane 0 by the overlap with W2, got %+v", lanes[1])
	}
}

// TestMergedClashMarkerAnchorNeverMoves covers the bug report: a
// clash marker's combined ID list (several events merged onto one
// column) can get long. It must never drag the o away from the
// column it actually marks — the label shifts side or clips instead —
// and it must never vanish outright when the label can't fit at all.
func TestMergedClashMarkerAnchorNeverMoves(t *testing.T) {
	tl := Timeline{
		Axis: AxisSymbolic,
		Windows: []Window{
			{ID: "W", Start: symPos(0), End: symPos(10)},
		},
		Events: []Event{
			{ID: "alpha", Time: symPos(9)},
			{ID: "bravo", Time: symPos(9)},
			{ID: "charlie", Time: symPos(9)},
			{ID: "delta", Time: symPos(9)},
		},
	}
	width := 40
	sc := newScale(tl, width)
	wantCol := sc.col(9)

	markers := buildEventMarkers(tl.Events, sc, Options{})
	lanes := packEventLanes(markers)
	row := []rune(eventMarkerRow(width, lanes[0]))

	if wantCol >= len(row) || row[wantCol] != 'o' {
		got := "?"
		if wantCol < len(row) {
			got = string(row[wantCol])
		}
		t.Fatalf("expected 'o' exactly at column %d regardless of label length (got %q), row=%q", wantCol, got, string(row))
	}
}

func TestBuildEventMarkersMergesByDefault(t *testing.T) {
	tl := Timeline{
		Axis: AxisSymbolic,
		Events: []Event{
			{ID: "E1", Time: symPos(2)},
			{ID: "E2", Time: symPos(2)},
		},
	}
	sc := newScale(tl, 40)

	markers := buildEventMarkers(tl.Events, sc, Options{})
	if len(markers) != 1 || len(markers[0].events) != 2 {
		t.Fatalf("expected one merged marker with 2 events, got %+v", markers)
	}

	markers = buildEventMarkers(tl.Events, sc, Options{StackClashingEvents: true})
	if len(markers) != 2 {
		t.Fatalf("expected 2 separate markers when stacking enabled, got %d", len(markers))
	}
}

func TestWindowLaneReservesIDWidth(t *testing.T) {
	// AA ends at col 5 (width 22 over span 0..15); BB starts at col 6
	// with a 2-char ID, so its label would land on AA's closing
	// bracket without the fix. They must land on separate lanes.
	tl := Timeline{
		Axis: AxisSymbolic,
		Windows: []Window{
			{ID: "AA", Start: symPos(0), End: symPos(5)},
			{ID: "BB", Start: symPos(6), End: symPos(10)},
		},
	}
	sc := newScale(tl, 22)
	lanes := packWindowLanes(tl.Windows, sc)
	if len(lanes) != 2 {
		t.Fatalf("got %d lanes, want 2 (BB's ID needs room AA's bracket doesn't leave)", len(lanes))
	}
}

func TestEventMarkerNearRightEdgeIsNotClipped(t *testing.T) {
	tl := Timeline{
		Axis: AxisSymbolic,
		Events: []Event{
			{ID: "E1", Label: "Late", Time: symPos(15)},
		},
	}
	out := Render(tl, 22)
	if !strings.Contains(out, "oE1") {
		t.Errorf("expected marker text 'oE1' to appear intact near the right edge, got:\n%s", out)
	}
}

// TestBackdatedMarkerAlignsWithRecordedColumn covers the bug report:
// a backdated event's o must land on the exact same column a plain
// event's o would occupy at that same time, in both arrow directions.
// A window widens the scale so neither column sits at the very edge
// of the canvas, keeping right-edge clamping out of the picture here.
func TestBackdatedMarkerAlignsWithRecordedColumn(t *testing.T) {
	cases := []struct {
		name           string
		time, recorded int
	}{
		{"forward", 20, 50},
		{"reversed", 50, 20},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tl := Timeline{
				Axis:    AxisSymbolic,
				Windows: []Window{{ID: "W", Start: symPos(0), End: symPos(100)}},
				Events:  []Event{{ID: "E1", Time: symPos(c.time), Backdated: true, RecordedAt: symPos(c.recorded)}},
			}
			width := 120
			sc := newScale(tl, width)
			markers := buildEventMarkers(tl.Events, sc, Options{})
			lanes := packEventLanes(markers)
			row := []rune(eventMarkerRow(width, lanes[0]))
			wantCol := sc.col(float64(c.recorded))

			plainRow := []rune(eventMarkerRow(width, []eventMarker{{col: wantCol, events: []Event{{ID: "P"}}}}))
			if plainRow[wantCol] != 'o' {
				t.Fatalf("sanity check failed: plain marker didn't land at col %d", wantCol)
			}

			if wantCol >= len(row) || row[wantCol] != 'o' {
				got := "?"
				if wantCol < len(row) {
					got = string(row[wantCol])
				}
				t.Fatalf("expected 'o' exactly at recorded column %d (got %q), row=%q", wantCol, got, string(row))
			}
		})
	}
}

func TestBackdatedEventSpansBothColumns(t *testing.T) {
	tl := Timeline{
		Axis: AxisSymbolic,
		Events: []Event{
			{ID: "E1", Time: symPos(1), Backdated: true, RecordedAt: symPos(5)},
		},
	}
	sc := newScale(tl, 40)
	markers := buildEventMarkers(tl.Events, sc, Options{})
	if len(markers) != 1 {
		t.Fatalf("expected 1 marker, got %d", len(markers))
	}
	m := markers[0]
	if m.effCol >= m.recCol {
		t.Fatalf("expected effCol < recCol when recorded after effective, got %d, %d", m.effCol, m.recCol)
	}
}

// TestBackdatedEventArrowDirection covers the bug report: when
// RecordedAt is *before* Time (recorded ahead of when it actually
// took effect), the arrow must still point from effective to
// recorded — i.e. leftward here — not just from the earlier column to
// the later one.
func TestBackdatedEventArrowDirection(t *testing.T) {
	tl := Timeline{
		Axis: AxisSymbolic,
		Events: []Event{
			{ID: "E1", Time: symPos(8), Backdated: true, RecordedAt: symPos(2)},
		},
	}
	sc := newScale(tl, 40)
	markers := buildEventMarkers(tl.Events, sc, Options{})
	m := markers[0]
	if m.effCol != sc.col(8) || m.recCol != sc.col(2) {
		t.Fatalf("effCol/recCol should track Time/RecordedAt regardless of order, got eff=%d rec=%d", m.effCol, m.recCol)
	}
	if m.recCol >= m.effCol {
		t.Fatalf("expected recCol < effCol for this reversed case, got rec=%d eff=%d", m.recCol, m.effCol)
	}

	out := Render(tl, 40)
	if !strings.Contains(out, "<") {
		t.Errorf("expected a left-pointing arrow when recorded_at precedes time, got:\n%s", out)
	}
	if strings.Contains(out, ">") {
		t.Errorf("did not expect a right-pointing arrow for this reversed case, got:\n%s", out)
	}
}
