package render

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type rawFile struct {
	Title   string      `yaml:"title"`
	Axis    Axis        `yaml:"axis"`
	Options rawOptions  `yaml:"options"`
	Windows []rawWindow `yaml:"windows"`
	Events  []rawEvent  `yaml:"events"`
}

type rawOptions struct {
	StackClashingEvents bool `yaml:"stack_clashing_events"`
}

type rawWindow struct {
	ID    string      `yaml:"id"`
	Label string      `yaml:"label"`
	Start interface{} `yaml:"start"`
	End   interface{} `yaml:"end"`
}

type rawEvent struct {
	ID       string      `yaml:"id"`
	Label    string      `yaml:"label"`
	Time     interface{} `yaml:"time"`
	LoggedAt interface{} `yaml:"logged_at"`
	Group    string      `yaml:"group"`
}

// LoadFile reads and parses a timeline definition from a YAML file.
// Only the first "---"-separated document is used; see LoadAllFile to
// render every document in a multi-document file.
func LoadFile(path string) (Timeline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Timeline{}, err
	}
	tl, err := Load(data)
	if err != nil {
		return Timeline{}, fmt.Errorf("%s: %w", path, err)
	}
	return tl, nil
}

// Load parses a timeline definition from YAML bytes. Only the first
// "---"-separated document is used; see LoadAll to render every
// document in multi-document input.
func Load(data []byte) (Timeline, error) {
	var raw rawFile
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return Timeline{}, fmt.Errorf("parse yaml: %w", err)
	}
	return buildTimeline(raw)
}

// LoadAllFile reads and parses every "---"-separated YAML document in
// a file, each an independent timeline definition.
func LoadAllFile(path string) ([]Timeline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	tls, err := LoadAll(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return tls, nil
}

// LoadAll parses every "---"-separated YAML document in data, each an
// independent timeline definition. A single-document input behaves
// exactly like Load, just wrapped in a one-element slice.
func LoadAll(data []byte) ([]Timeline, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var timelines []Timeline
	for i := 1; ; i++ {
		var raw rawFile
		if err := dec.Decode(&raw); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("document %d: parse yaml: %w", i, err)
		}
		tl, err := buildTimeline(raw)
		if err != nil {
			return nil, fmt.Errorf("document %d: %w", i, err)
		}
		timelines = append(timelines, tl)
	}
	if len(timelines) == 0 {
		return nil, fmt.Errorf("no YAML documents found")
	}
	return timelines, nil
}

// buildTimeline validates a parsed document and assembles the
// Timeline it describes.
func buildTimeline(raw rawFile) (Timeline, error) {
	switch raw.Axis {
	case AxisAbsolute, AxisSymbolic:
	case "":
		return Timeline{}, fmt.Errorf("axis is required (must be %q or %q)", AxisAbsolute, AxisSymbolic)
	default:
		return Timeline{}, fmt.Errorf("unknown axis %q (must be %q or %q)", raw.Axis, AxisAbsolute, AxisSymbolic)
	}

	tl := Timeline{
		Title:   raw.Title,
		Axis:    raw.Axis,
		Options: Options{StackClashingEvents: raw.Options.StackClashingEvents},
	}

	for i, w := range raw.Windows {
		name := labelOrID(w.ID, w.Label)
		start, err := decodePos(w.Start, raw.Axis)
		if err != nil {
			return Timeline{}, fmt.Errorf("window %d (%s): start: %w", i, name, err)
		}
		end, err := decodePos(w.End, raw.Axis)
		if err != nil {
			return Timeline{}, fmt.Errorf("window %d (%s): end: %w", i, name, err)
		}
		if end.float() < start.float() {
			return Timeline{}, fmt.Errorf("window %d (%s): end before start", i, name)
		}
		id := w.ID
		if id == "" {
			id = windowAutoID(i)
		}
		tl.Windows = append(tl.Windows, Window{ID: id, Label: w.Label, Start: start, End: end})
	}

	for i, e := range raw.Events {
		name := labelOrID(e.ID, e.Label)
		t, err := decodePos(e.Time, raw.Axis)
		if err != nil {
			return Timeline{}, fmt.Errorf("event %d (%s): time: %w", i, name, err)
		}
		id := e.ID
		if id == "" {
			id = fmt.Sprintf("E%d", i+1)
		}
		ev := Event{ID: id, Label: e.Label, Time: t, Group: e.Group}
		if e.LoggedAt != nil {
			r, err := decodePos(e.LoggedAt, raw.Axis)
			if err != nil {
				return Timeline{}, fmt.Errorf("event %d (%s): logged_at: %w", i, name, err)
			}
			ev.Backdated = true
			ev.LoggedAt = r
		}
		tl.Events = append(tl.Events, ev)
	}

	if len(tl.Windows) == 0 && len(tl.Events) == 0 {
		return Timeline{}, fmt.Errorf("timeline has no windows or events")
	}

	return tl, nil
}

func decodePos(v interface{}, axis Axis) (pos, error) {
	if axis == AxisSymbolic {
		switch n := v.(type) {
		case int:
			return symPos(n), nil
		case int64:
			return symPos(int(n)), nil
		default:
			return pos{}, fmt.Errorf("expected integer for symbolic axis, got %T", v)
		}
	}
	switch t := v.(type) {
	case time.Time:
		return absPos(t), nil
	case string:
		parsed, err := time.Parse(time.RFC3339, t)
		if err != nil {
			return pos{}, fmt.Errorf("expected RFC3339 timestamp for absolute axis: %w", err)
		}
		return absPos(parsed), nil
	default:
		return pos{}, fmt.Errorf("expected RFC3339 timestamp for absolute axis, got %T", v)
	}
}

func labelOrID(id, label string) string {
	if id != "" {
		return id
	}
	if label != "" {
		return label
	}
	return "?"
}

// windowAutoID produces spreadsheet-style column labels: A, B, ..., Z,
// AA, AB, ... for windows whose id was omitted.
func windowAutoID(i int) string {
	i++ // 1-based
	s := ""
	for i > 0 {
		i--
		s = string(rune('A'+i%26)) + s
		i /= 26
	}
	return s
}
