// Package tokens provides a deterministic, dependency-free token estimate for
// logmind's token-efficiency surfaces (`logmind context --stats`, and future
// `gain`/receipt output). It is the toolchain's shared estimator per the
// protocol § Token-efficiency contract: the common ~4-bytes-per-token proxy,
// never a billed count — just a stable, no-network approximation so token
// savings are auditable the same way across tools.
package tokens

// Estimate returns a rough token count for s using ceil(len(s)/4) — the widely
// used ~4-bytes-per-token heuristic. Deterministic; makes no network/LLM call.
// It intentionally counts bytes (not runes): token count tracks bytes closely
// for English/code, and byte-length keeps the estimate stable and cheap.
func Estimate(s string) int {
	if len(s) == 0 {
		return 0
	}
	return (len(s) + 3) / 4
}
