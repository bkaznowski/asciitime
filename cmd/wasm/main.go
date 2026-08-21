//go:build js && wasm

// Command wasm exposes render.LoadAll and render.Render to JavaScript
// as a single global function, window.asciitimeRender(yamlText, width),
// for the static GitHub Pages front end in docs/. Multiple
// "---"-separated YAML documents render as multiple diagrams, joined
// the same way the CLI joins them.
package main

import (
	"strings"
	"syscall/js"

	"github.com/bkaznowski/asciitime/internal/render"
)

func renderYAML(this js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return jsResult("", "missing yaml argument")
	}
	yamlText := args[0].String()

	width := 0
	if len(args) > 1 && args[1].Type() == js.TypeNumber {
		width = args[1].Int()
	}

	timelines, err := render.LoadAll([]byte(yamlText))
	if err != nil {
		return jsResult("", err.Error())
	}

	diagrams := make([]string, len(timelines))
	for i, tl := range timelines {
		diagrams[i] = render.Render(tl, width)
	}
	return jsResult(strings.Join(diagrams, "\n\n---\n\n"), "")
}

func jsResult(output, errMsg string) map[string]interface{} {
	return map[string]interface{}{
		"output": output,
		"error":  errMsg,
	}
}

func main() {
	js.Global().Set("asciitimeRender", js.FuncOf(renderYAML))
	select {} // keep the program alive so JS can keep calling into it
}
