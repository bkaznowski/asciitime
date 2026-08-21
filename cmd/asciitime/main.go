// Command asciitime renders a YAML timeline definition as an ASCII
// diagram. A file containing multiple "---"-separated YAML documents
// renders each as its own diagram.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/bkaznowski/asciitime/internal/render"
)

func main() {
	width := flag.Int("width", 0, "target output width in columns (default: autofit to 78)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s [-width N] <file.yaml>\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	timelines, err := render.LoadAllFile(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "asciitime:", err)
		os.Exit(1)
	}

	diagrams := make([]string, len(timelines))
	for i, tl := range timelines {
		diagrams[i] = render.Render(tl, *width)
	}

	fmt.Println(strings.Join(diagrams, "\n\n---\n\n"))
}
