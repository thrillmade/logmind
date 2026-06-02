// Command logmind is the entry point for the logmind CLI binary.
//
// Wave B1.scaffold: this just hands control to internal/cli. All command
// wiring lives there so the binary stays a thin shim — keeps the cmd/
// directory minimal as the rewrite grows through subsequent waves.
package main

import (
	"fmt"
	"os"

	"github.com/thrillmade/logmind/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		// cobra already prints its own errors; emit a fallback line so
		// non-cobra failures (rare here, but possible from RunE returns)
		// still surface on stderr before we exit non-zero.
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
