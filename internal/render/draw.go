package render

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

func (m eventMarker) isBackdated() bool {
	return len(m.events) == 1 && m.events[0].Backdated
}

// idList is the comma-joined IDs of a point (non-backdated) marker —
// more than one when several events were merged onto the same column.
func (m eventMarker) idList() string {
	ids := make([]string, len(m.events))
	for i, e := range m.events {
		ids[i] = e.ID
	}
	return strings.Join(ids, ",")
}

func drawRow(width int, fn func(row []rune)) string {
	row := make([]rune, width)
	for i := range row {
		row[i] = ' '
	}
	fn(row)
	return strings.TrimRight(string(row), " ")
}

func placeText(row []rune, col int, text string) {
	for i, r := range []rune(text) {
		c := col + i
		if c < 0 || c >= len(row) {
			continue
		}
		row[c] = r
	}
}

// clampedStart computes where a textLen-wide label will actually land:
// col, shifted left just far enough to stay within width if it would
// otherwise run off the right edge, and right past nextFree if that
// shift would run it into whatever was drawn immediately before.
// Callers that need to know the real landing spot before drawing
// anything else around it (so a fixed-position neighbor like a dash
// fill doesn't assume the label landed exactly at col) should call
// this directly instead of placeClamped.
func clampedStart(col, textLen, width, nextFree int) int {
	start := col
	if start+textLen > width {
		start = width - textLen
	}
	if start < 0 {
		start = 0
	}
	if start < nextFree {
		start = nextFree
	}
	return start
}

// placeClamped draws text starting at col, shifting it via
// clampedStart if needed to stay on-canvas and clear of nextFree. If
// even the clamped position doesn't leave room for the whole label,
// it is dropped rather than silently truncated mid glyph. Returns the
// nextFree cursor for the next call in the row.
func placeClamped(row []rune, col int, text string, width, nextFree int) int {
	r := []rune(text)
	start := clampedStart(col, len(r), width, nextFree)
	if start < 0 || start+len(r) > width {
		return nextFree
	}
	placeText(row, start, text)
	return start + len(r) + 1
}

// placeAnchoredLabel draws a single-character anchor at col
// unconditionally — the anchor marks an exact time position and must
// never move, no matter how long the accompanying label is — plus a
// label right next to it. The label goes on the preferred side
// (preferRight) if it fits there without running off the canvas or
// into whatever nextFree marks as already occupied; otherwise it
// tries the other side; if it fits on neither, it's drawn on the
// preferred side anyway and simply clipped at the canvas edge, same
// as any other out-of-bounds text. The anchor itself is unaffected
// either way.
//
// Returns the full [start, end] column span actually drawn into (so a
// caller filling in a connector on one side, e.g. the dashed line to
// a backdated marker's ◇, can route around wherever the label really
// landed rather than assuming its ideal position) and the updated
// nextFree cursor.
func placeAnchoredLabel(row []rune, col int, anchor, label string, width, nextFree int, preferRight bool) (occupiedStart, occupiedEnd, newNextFree int) {
	placeText(row, col, anchor)
	anchorLen := utf8.RuneCountInString(anchor)
	labelLen := utf8.RuneCountInString(label)

	rightStart := col + anchorLen
	leftStart := col - labelLen
	fitsRight := rightStart >= nextFree && rightStart+labelLen <= width
	fitsLeft := leftStart >= nextFree && leftStart >= 0

	var start int
	if preferRight {
		switch {
		case fitsRight:
			start = rightStart
		case fitsLeft:
			start = leftStart
		default:
			start = rightStart // neither fits — clip at the right edge
		}
	} else {
		switch {
		case fitsLeft:
			start = leftStart
		case fitsRight:
			start = rightStart
		default:
			start = leftStart // neither fits — clip at the left edge
		}
	}
	placeText(row, start, label)

	occupiedStart = col
	if start < occupiedStart {
		occupiedStart = start
	}
	occupiedEnd = col + anchorLen - 1
	if labelEnd := start + labelLen - 1; labelEnd > occupiedEnd {
		occupiedEnd = labelEnd
	}
	newNextFree = occupiedEnd + 2
	return occupiedStart, occupiedEnd, newNextFree
}

func windowRow(width int, lane []Window, sc scale) string {
	return drawRow(width, func(row []rune) {
		for _, w := range lane {
			startCol, endCol := sc.col(w.Start.float()), sc.col(w.End.float())
			for c := startCol; c <= endCol; c++ {
				switch c {
				case startCol:
					row[c] = '['
				case endCol:
					row[c] = ']'
				default:
					row[c] = '='
				}
			}
			idCol := startCol - len([]rune(w.ID))
			if idCol < 0 {
				// No room to the left of the bracket (window starts at
				// column 0) — drop the ID just inside it instead.
				idCol = startCol + 1
			}
			placeText(row, idCol, w.ID)
		}
	})
}

// eventMarkerRow draws each marker in a lane. Every anchor glyph — a
// plain event's ●, a backdated event's ◇ and ● — is placed at its
// exact time column unconditionally; only the ID label next to it can
// shift (to the other side) or clip to stay on-canvas. This matters
// most for merged clash markers, whose comma-joined ID list can get
// long enough to otherwise run off the canvas — the label handles
// that by shifting or clipping, never by dragging the point away from
// the time it actually marks.
//
// A backdated marker's ◇ sits at its effective-time column and its ●
// at its recorded-time column, with an arrowhead in the gap between
// them (dropped if the gap is too narrow to fit one) pointing from
// effective to recorded regardless of which falls earlier on the
// axis, so a "recorded before it happened" case points left instead
// of silently pointing right like the rest.
func eventMarkerRow(width int, lane []eventMarker) string {
	return drawRow(width, func(row []rune) {
		nextFree := 0
		for _, m := range lane {
			if m.isBackdated() {
				id := m.events[0].ID
				if m.effCol <= m.recCol {
					// Label prefers the right, away from the diamond, so
					// it normally lands exactly at m.recCol with nothing
					// between it and effCol but the connector. Compute
					// the connector from where it actually landed, not
					// that ideal, in case a canvas-edge fallback pushed
					// it toward the diamond instead.
					occupiedStart, _, nf := placeAnchoredLabel(row, m.recCol, "●", id, width, nextFree, true)
					nextFree = nf
					gapEnd := occupiedStart - 1
					if gapEnd > m.effCol {
						for c := m.effCol + 1; c < gapEnd; c++ {
							row[c] = '╌'
						}
						placeText(row, gapEnd, "▶")
					}
					placeText(row, m.effCol, "◇")
				} else {
					_, occupiedEnd, nf := placeAnchoredLabel(row, m.recCol, "●", id, width, nextFree, false)
					nextFree = nf
					gapStart := occupiedEnd + 1
					if gapStart < m.effCol {
						for c := gapStart + 1; c < m.effCol; c++ {
							row[c] = '╌'
						}
						placeText(row, gapStart, "◀")
					}
					placeText(row, m.effCol, "◇")
				}
			} else {
				_, _, nf := placeAnchoredLabel(row, m.col, "●", m.idList(), width, nextFree, true)
				nextFree = nf
			}
		}
	})
}

// tickLabelRow places each tick's label starting at its column, except
// where that would run past the right edge or into the previous
// label — then it shifts right just enough to clear the previous
// label, or clamps left so it still fits within width. A label that
// still can't fit either way is dropped; its '+' mark in the rule row
// still shows its position.
func tickLabelRow(width int, ticks []tick) string {
	return drawRow(width, func(row []rune) {
		nextFree := 0
		for _, t := range ticks {
			nextFree = placeClamped(row, t.col, t.label, width, nextFree)
		}
	})
}

func tickRuleRow(width int, ticks []tick) string {
	return drawRow(width, func(row []rune) {
		for c := range row {
			row[c] = '-'
		}
		for _, t := range ticks {
			row[t.col] = '+'
		}
	})
}

func formatPos(axis Axis, p pos) string {
	if axis == AxisSymbolic {
		return fmt.Sprintf("T%d", p.sym)
	}
	return p.abs.Format("2006-01-02 15:04")
}

func legendLines(tl Timeline) []string {
	idWidth, labelWidth := 0, 0
	grow := func(id, label string) {
		if len([]rune(id)) > idWidth {
			idWidth = len([]rune(id))
		}
		if len([]rune(label)) > labelWidth {
			labelWidth = len([]rune(label))
		}
	}
	for _, w := range tl.Windows {
		grow(w.ID, w.Label)
	}
	for _, e := range tl.Events {
		grow(e.ID, e.Label)
	}

	var lines []string
	for _, w := range tl.Windows {
		lines = append(lines, fmt.Sprintf("%-*s  %-*s  %s -> %s",
			idWidth, w.ID, labelWidth, w.Label, formatPos(tl.Axis, w.Start), formatPos(tl.Axis, w.End)))
	}
	for _, e := range tl.Events {
		when := formatPos(tl.Axis, e.Time)
		if e.Backdated {
			when = fmt.Sprintf("effective %s, recorded %s", formatPos(tl.Axis, e.Time), formatPos(tl.Axis, e.RecordedAt))
		}
		lines = append(lines, fmt.Sprintf("%-*s  %-*s  %s", idWidth, e.ID, labelWidth, e.Label, when))
	}
	return lines
}
