// Command logmind is the entry point for the logmind CLI binary.
//
// Wave B1.scaffold: this just hands control to internal/cli. All command
// wiring lives there so the binary stays a thin shim — keeps the cmd/
// directory minimal as the rewrite grows through subsequent waves.
package main

import (
	"os"

	"github.com/thrillmade/logmind/internal/cli"
)

func main() {
	// cobra (with SilenceErrors=false, the default) already prints the
	// returned error to stderr via PrintErrln before Execute returns.
	// We only need to set the non-zero exit code here — any extra
	// fmt.Fprintln would duplicate the error message on stderr.
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
