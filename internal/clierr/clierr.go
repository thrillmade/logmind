// Package clierr holds error sentinels that cross internal-package
// boundaries (skill, cli, ...). Keeping them here breaks what would
// otherwise be a circular dependency: `internal/skill` needs to wrap
// the "silent exit-1" sentinel its errors carry through to the CLI
// layer, while `internal/cli` needs to recognise that sentinel to
// trigger cobra's silent-exit behaviour. Both packages import this
// one; neither imports the other for this purpose.
//
// Why a new tiny package and not a moveinto either side: the skill
// package owns the production code that decides "this is a user-input
// error worth showing the message and exiting silently", and the cli
// package owns the cobra wiring that translates that signal into a
// non-zero exit. They share the SENTINEL but not the responsibility
// — so the sentinel lives outside both, mirroring how stdlib pulls
// `io.EOF` out of any particular reader/writer implementation.
package clierr

import "errors"

// ErrSilent signals "exit non-zero without re-printing the error via
// cobra". The user-facing message has already been written to stdout
// (matches Python's sys.exit(1) after a click.echo); the cobra layer's
// SilenceErrors + main()'s os.Exit(1)-on-non-nil-Execute together
// produce a byte-identical shape.
//
// Wrapping convention: any sentinel that should trigger silent-exit
// is built via `fmt.Errorf("%w: ...", clierr.ErrSilent)` so callers
// can recover the underlying "user-input error" with errors.Is without
// having to switch on every concrete sentinel by hand.
var ErrSilent = errors.New("logmind: exit 1")
