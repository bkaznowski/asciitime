// Command asciitime renders a YAML timeline definition as an ASCII
// diagram.
package main

import (
	"flag"
	"fmt"
	"os"

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

	tl, err := render.LoadFile(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "asciitime:", err)
		os.Exit(1)
	}

	fmt.Println(render.Render(tl, *width))
}
