// Command logmind is the entry point for the logmind CLI binary.
//
// Wave B1.scaffold: this just hands control to internal/cli. All command
// wiring lives there so the binary stays a thin shim — keeps the cmd/
// directory minimal as the rewrite grows through subsequent waves.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/thrillmade/logmind/internal/cli"
)

func main() {
	err := cli.Execute()
	if err == nil {
		return
	}
	// The cobra root has SilenceErrors=true (root.go) so RunE handlers
	// that already printed a user-facing message via cmd.Print* return
	// cli.ErrSilent. We exit non-zero without saying anything more —
	// matches Python v0.6.14 byte-for-byte (sys.exit(1) shape).
	//
	// Unexpected/real errors (mkdir denied, chmod failed, etc.) come
	// through as plain errors. cobra suppressed them; if main also
	// stays silent, the user gets no diagnostic. Print to stderr so
	// genuine failures surface instead of vanishing.
	if !errors.Is(err, cli.ErrSilent) {
		fmt.Fprintln(os.Stderr, "Error:", err)
	}
	os.Exit(1)
}
