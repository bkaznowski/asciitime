package render

import "strings"

const defaultWidth = 78

// Render produces the full ASCII diagram for tl: title, autofit axis
// header, window lanes, event markers, and a legend. width <= 0 uses a
// sensible default.
func Render(tl Timeline, width int) string {
	if width <= 0 {
		width = defaultWidth
	}

	sc := newScale(tl, width)
	ticks := buildTicks(tl, sc)

	var b strings.Builder
	if tl.Title != "" {
		b.WriteString(tl.Title)
		b.WriteString("\n\n")
	}

	b.WriteString(tickLabelRow(width, ticks))
	b.WriteString("\n")
	b.WriteString(tickRuleRow(width, ticks))
	b.WriteString("\n")

	for _, lane := range packWindowLanes(tl.Windows, sc) {
		b.WriteString(windowRow(width, lane, sc))
		b.WriteString("\n")
	}

	anyGrouped := false
	for _, e := range tl.Events {
		if e.Group != "" {
			anyGrouped = true
			break
		}
	}

	for _, g := range groupEvents(tl.Events) {
		if anyGrouped {
			b.WriteString("\n")
			if g.name != "" {
				b.WriteString(g.name)
				b.WriteString("\n")
			}
		}
		markers := buildEventMarkers(g.events, sc, tl.Options)
		for _, lane := range packEventLanes(markers) {
			b.WriteString(eventMarkerRow(width, lane))
			b.WriteString("\n")
		}
	}

	if legend := legendLines(tl); len(legend) > 0 {
		b.WriteString("\nLegend\n")
		for _, l := range legend {
			b.WriteString(l)
			b.WriteString("\n")
		}
	}

	return strings.TrimRight(b.String(), "\n")
}
