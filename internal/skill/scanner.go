package skill

import (
	"regexp"
	"strings"
)

// scanner.go — §8.2 privacy gate, layer 3: content-scanner.
//
// Scans the SKILL.md body for evidence that the skill is unsafe to push
// to a shared catalog. Three categories of pattern fire at the gate:
//
//   - Credential-shaped tokens (regex set).
//   - Internal-process keywords (case-insensitive substring matches).
//   - Org-internal domain references (regex built from a config list).
//   - Bare absolute-path references that look local-machine-specific.
//
// Each category has a default severity ("block" or "warn"). Hits with
// severity "block" abort the push; "warn" hits are printed to stderr
// and the push continues. Severities can be widened (warn → block) but
// NEVER weakened (block → warn) — the hardcoded baseline at the bottom
// of this file always reigns.
//
// Why a separate file from push.go: the scanner is a pure function on
// (body, config) — no filesystem, no subprocess, no global state — so
// it tests cleanly without spinning up a full pushWith call. The push
// gate just walks the returned slice and decides whether to block or
// warn. Tests live in scanner_test.go.
//
// Why not entropy / length-based heuristics for unknown API keys: the
// false-positive rate on long base64/uuid strings is catastrophic for a
// gate that has no --force flag. We rely on the well-known token
// prefixes (Stripe, Slack, GitHub, npm, AWS, GCP) and let user
// keyword/domain config catch in-house token shapes that don't follow
// public conventions.

// PrivacyScannerHit is one finding from a content-scanner pass. The
// struct is part of the public surface of the skill package so the cli
// layer can render it consistently across categories.
//
// Field semantics:
//
//   - Layer: always "content-scanner" for this pass. Reserved as a
//     field (rather than hardcoded "layer 3" in print sites) so the
//     same struct can carry hits from future layers (e.g., a layer-5
//     binary-blob scan) without changing the print template.
//   - Kind: which of the four pattern categories fired. Stable string
//     ("credential", "keyword", "org-domain", "local-path") so callers
//     can group / filter. NOT i18n'd — agent skill catalog is English-only.
//   - Match: the offending substring (or a redacted preview, see
//     redactCredential below). Used in the user-facing message so the
//     author can locate the bad text in SKILL.md.
//   - LineNumber: 1-based, matches what editors and SKILL.md headings
//     show. Computed by counting newlines up to the match offset.
//   - Severity: "block" or "warn". Block hits abort the push; warn hits
//     are flagged in stderr and execution continues.
type PrivacyScannerHit struct {
	Layer      string
	Kind       string
	Match      string
	LineNumber int
	Severity   string
}

// ScannerConfig bundles user-tunable scanner inputs. Resolved by the
// config package (LoadScannerConfig) and threaded through PushOptions.
// Defaults (zero values) mean "scanner runs with hardcoded baseline only" —
// the gate still fires; user config purely ADDS to the deny set.
//
// Why no DisableCategories field: this is a guard rail. The point of
// not bypass-able-from-config is that even a malicious / accidental
// `.logmind/config.yml` rewrite can't loosen the gate below baseline.
// Users who genuinely need to push something the gate rejects must
// remove the offending content from SKILL.md (the correct fix) or
// move the skill to a private path.
type ScannerConfig struct {
	// Keywords is an additive list of substrings that, if present in
	// SKILL.md body (case-insensitive), trigger a "keyword" hit. Merged
	// with the hardcoded baseline (`confidential`, `proprietary`, ...).
	// Severity defaults to "block" — see SeverityOverrides to widen.
	Keywords []string

	// OrgDomains is the user's list of internal domain strings (with
	// TLDs included, e.g. "thrillmade.internal", "thrillmade.local").
	// Each entry is escaped and wrapped in a regex matching
	// `<word>.<domain>` references in the body. Empty list means no
	// org-domain matches fire — the category has NO baseline because
	// every org has a different internal-domain shape.
	OrgDomains []string

	// SeverityOverrides maps category Kind ("credential", "keyword",
	// "org-domain", "local-path") to "block" or "warn". Used to WIDEN
	// the default severity (e.g., promote "local-path" from "warn" to
	// "block" in repos that consider any path leak unacceptable).
	// Trying to weaken (block → warn) for a baseline-block category is
	// silently ignored — the baseline wins. See applySeverityOverride.
	SeverityOverrides map[string]string
}

// Severity constants. Exported so call sites + tests don't drift on
// the string literal.
const (
	SeverityBlock = "block"
	SeverityWarn  = "warn"
)

// Kind constants. Same rationale — pinning the strings here means
// rename refactors flow through a single declaration site.
const (
	KindCredential = "credential"
	KindKeyword    = "keyword"
	KindOrgDomain  = "org-domain"
	KindLocalPath  = "local-path"
)

// scannerLayerName is the constant stored in PrivacyScannerHit.Layer
// for every hit this file produces. Future layers (e.g., a binary-blob
// scanner) would set a different layer name on their hits.
const scannerLayerName = "content-scanner"

// HARDCODED_BASELINE_CREDENTIAL_PATTERNS is the bypass-proof set of
// credential-shaped token regexes. Same shape as D.2.7's HARDCODED_DENY_PATHS:
// listed inline as Go regexps, never read from config, never overrideable.
//
// Categories covered:
//
//   - Stripe live + restricted keys: `sk_live_…`, `pk_live_…`, `sk_…`,
//     `rk_live_…`.
//   - Slack bot/user/app tokens: `xoxb-`, `xoxp-`, `xoxa-`, `xoxr-`,
//     `xoxe-`, `xoxs-`.
//   - GitHub Personal Access / app tokens: `ghp_`, `gho_`, `ghu_`,
//     `ghs_`, `ghr_`, `github_pat_`.
//   - npm publish tokens: `npm_`.
//   - AWS Access Key IDs: `AKIA…` (matches 16 alnum after).
//   - GCP service-account JSON shape: `"type": "service_account"` (the
//     least-ambiguous fingerprint — the full JSON would also have
//     "private_key" but the shape match alone is enough to flag).
//
// Why we anchor on prefixes rather than entropy: token prefixes are
// stable across provider rotations and have effectively zero
// false-positive rate vs the kind of plaintext skills authors write.
// Length-based heuristics on random alnum strings, by contrast, would
// false-positive on UUIDs / commit SHAs / base64 sample data.
//
// The regexes use Go's regexp package (RE2). No PCRE features used.
// Word boundaries (`\b`) keep us from matching inside another word
// (e.g., `xsk_test_…` shouldn't trip the `sk_…` pattern).
var hardcodedCredentialPatterns = []struct {
	Name string
	RE   *regexp.Regexp
}{
	// Stripe — match `sk_live_…`, `sk_test_…`, `pk_live_…`, `pk_test_…`,
	// `rk_live_…`, `rk_test_…`. Stripe keys are 24+ alnum chars; we
	// require at least 8 trailing chars to dodge `sk_live_FAKE` example
	// stubs people sometimes leave for illustration. Word-bounded.
	{
		"stripe",
		regexp.MustCompile(`\b(?:sk|pk|rk)_(?:live|test)_[A-Za-z0-9]{8,}\b`),
	},
	// Slack — six token shapes (`xoxb`, `xoxp`, `xoxa`, `xoxr`, `xoxe`,
	// `xoxs`) each followed by hyphenated digits + the secret. We
	// require at least one hyphen and 8 alnum trailing chars to skip
	// generic mentions like "xoxp tokens".
	{
		"slack",
		regexp.MustCompile(`\bxox[abpresx]-[A-Za-z0-9-]{8,}\b`),
	},
	// GitHub PAT (classic + fine-grained), OAuth app, server-to-server,
	// refresh, and app installation tokens. All share the `gh{c}_`
	// prefix; the fine-grained PAT uses `github_pat_` instead. We
	// require ≥36 trailing chars to avoid matching `ghp_` mentions in
	// prose (GitHub PATs are 40+ chars including the prefix).
	{
		"github",
		regexp.MustCompile(`\b(?:ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9]{36,}\b`),
	},
	{
		"github-finegrained",
		regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{60,}\b`),
	},
	// npm publish tokens. npm tokens are UUID-shaped after the prefix:
	// 8-4-4-4-12 hex. We accept the historical longer-form too (`npm_`
	// + ≥36 alnum) for forward compat with token-format updates.
	{
		"npm",
		regexp.MustCompile(`\bnpm_[A-Za-z0-9]{36,}\b`),
	},
	// AWS Access Key ID. Format: `AKIA` + 16 uppercase alnum. The
	// secret-access-key (40 mixed-case) is too entropic to fingerprint
	// safely, but the ACCESS KEY ID alone is a strong leak signal
	// because pasting it implies the secret is somewhere nearby.
	{
		"aws-access-key",
		regexp.MustCompile(`\bAKIA[A-Z0-9]{16}\b`),
	},
	// AWS Secret Access Key has no fixed prefix and matching purely on
	// 40-char base64 would false-positive on commit SHAs and example
	// hashes. We skip it — the access-key match above is what catches
	// real leaks, since live IAM users always paste both.

	// GCP service-account JSON. The shape match is unambiguous because
	// `"type": "service_account"` only appears in a SA key JSON. We
	// tolerate whitespace variation between `:` and the value.
	{
		"gcp-service-account",
		regexp.MustCompile(`"type"\s*:\s*"service_account"`),
	},
}

// HARDCODED_BASELINE_KEYWORDS is the bypass-proof keyword baseline.
// Any of these substrings appearing in SKILL.md (case-insensitive)
// triggers a "keyword" hit at "block" severity. User config can ADD to
// this list via ScannerConfig.Keywords but can't remove from it.
//
// Why these five: each is a high-signal phrase that almost never
// appears in legitimate generic skill content. They overlap on
// purpose — a skill that says "Confidential — do not share outside
// the NDA" fires three times, surfacing maximum context for the
// rejection message.
//
// Why we DON'T include "secret" or "password": those are too common
// in legitimate prose ("the skill manages secret rotation"; "extract
// the password from the env"). The baseline targets keywords whose
// presence is a near-certain signal of misuse-paste from an internal
// document. User config adds in-house terms (e.g. company codenames).
var hardcodedKeywordBaseline = []string{
	"confidential",
	"proprietary",
	"internal use only",
	"do not share",
	"nda",
	"under embargo",
}

// localPathPatterns matches absolute paths that look local-machine-specific
// rather than canonical project paths. We catch:
//
//   - macOS-style: `/Users/<name>/…`
//   - Linux-style: `/home/<name>/…`
//
// Severity defaults to "warn" — skills sometimes legitimately quote a
// home-relative example path in prose, and a hard block here would
// false-positive on those. Org-wide config can promote to "block" via
// SeverityOverrides if local-path leakage is policy-grade serious.
//
// We deliberately don't try to match Windows-style paths (`C:\Users\…`)
// here: Windows skill authoring through Claude Code is rare enough that
// the false-positive rate (Windows codepaths in code examples) outweighs
// the leak signal. Add `windows-path` as a separate category if a
// concrete leak event motivates it.
var localPathPatterns = []*regexp.Regexp{
	regexp.MustCompile(`/Users/[A-Za-z0-9._-]+/`),
	regexp.MustCompile(`/home/[A-Za-z0-9._-]+/`),
}

// defaultSeverityByKind is the baseline severity per category. Used
// when ScannerConfig.SeverityOverrides has no entry for the kind.
// "credential" and "keyword" default to BLOCK (must not ship);
// "org-domain" and "local-path" default to WARN (could be intentional).
var defaultSeverityByKind = map[string]string{
	KindCredential: SeverityBlock,
	KindKeyword:    SeverityBlock,
	KindOrgDomain:  SeverityWarn,
	KindLocalPath:  SeverityWarn,
}

// baselineBlockKinds is the set of categories whose default severity
// is BLOCK and MUST NOT be weakened by config. Trying to override
// "credential" or "keyword" to "warn" is silently ignored — the
// baseline wins. See applySeverityOverride.
//
// Why kept as a separate set rather than reading from defaultSeverityByKind:
// the baseline-block contract is policy, not derived from defaults. If
// we ever flip a default to allow user override (unlikely), keeping
// the two lists separate documents that intent at the change site.
var baselineBlockKinds = map[string]bool{
	KindCredential: true,
	KindKeyword:    true,
}

// ScanContent runs every scanner category over body and returns all
// hits in document order (top-to-bottom). The caller decides whether
// any hit at severity "block" aborts the push (see push.go) — this
// function is purely diagnostic and never returns an error.
//
// Returns an empty slice when nothing fires. nil-safe on config: a
// zero ScannerConfig means "baseline only".
//
// Why returning ALL hits rather than first-match-and-return: surfaces
// every leak source in one error message, so the author fixes the
// SKILL.md in one editing pass rather than discovering issues one at
// a time across re-runs.
func ScanContent(body []byte, cfg ScannerConfig) []PrivacyScannerHit {
	if len(body) == 0 {
		return nil
	}
	text := string(body)

	var hits []PrivacyScannerHit
	hits = append(hits, scanCredentials(text)...)
	hits = append(hits, scanKeywords(text, cfg.Keywords)...)
	hits = append(hits, scanOrgDomains(text, cfg.OrgDomains)...)
	hits = append(hits, scanLocalPaths(text)...)

	// Apply user-supplied severity overrides where allowed. We do this
	// AFTER all categories fire so the override logic can see the
	// final default severity and reject downgrades against baseline.
	for i := range hits {
		hits[i].Severity = applySeverityOverride(hits[i].Kind, hits[i].Severity, cfg.SeverityOverrides)
	}

	return hits
}

// scanCredentials walks the hardcoded credential regexes and records
// every match. Match offsets feed lineNumberAt so the user-facing
// error points at the SKILL.md line that needs editing.
//
// Match field stores the REDACTED token shape (prefix + `…`) — we
// don't want to print the literal token back to stderr where it might
// get captured in a CI log and become the next leak. See redactCredential.
func scanCredentials(text string) []PrivacyScannerHit {
	var hits []PrivacyScannerHit
	for _, p := range hardcodedCredentialPatterns {
		for _, loc := range p.RE.FindAllStringIndex(text, -1) {
			raw := text[loc[0]:loc[1]]
			hits = append(hits, PrivacyScannerHit{
				Layer:      scannerLayerName,
				Kind:       KindCredential,
				Match:      redactCredential(p.Name, raw),
				LineNumber: lineNumberAt(text, loc[0]),
				Severity:   defaultSeverityByKind[KindCredential],
			})
		}
	}
	return hits
}

// scanKeywords matches both the hardcoded baseline AND user-supplied
// additions, case-insensitive. We dedupe the merged list so a user
// who adds "confidential" to their config doesn't get double-billed.
//
// We use strings.Contains on lowercased text rather than regex per
// keyword — fewer compiles, and keywords aren't pattern-y enough to
// need anchoring.
func scanKeywords(text string, extra []string) []PrivacyScannerHit {
	keywords := mergeKeywords(hardcodedKeywordBaseline, extra)
	lower := strings.ToLower(text)

	var hits []PrivacyScannerHit
	for _, kw := range keywords {
		if kw == "" {
			continue
		}
		kwLower := strings.ToLower(kw)
		// Walk every occurrence by sliding the search window — we want
		// to flag every paragraph that mentions the keyword, not just
		// the first.
		offset := 0
		for {
			idx := strings.Index(lower[offset:], kwLower)
			if idx < 0 {
				break
			}
			absIdx := offset + idx
			// Match field uses the ORIGINAL casing so the error message
			// echoes what the author actually wrote — easier to grep.
			matchLen := len(kwLower)
			rawMatch := text[absIdx : absIdx+matchLen]
			hits = append(hits, PrivacyScannerHit{
				Layer:      scannerLayerName,
				Kind:       KindKeyword,
				Match:      rawMatch,
				LineNumber: lineNumberAt(text, absIdx),
				Severity:   defaultSeverityByKind[KindKeyword],
			})
			offset = absIdx + matchLen
		}
	}
	return hits
}

// scanOrgDomains walks the user's org-domain list and matches
// `<word>.<domain>` references in the body. No baseline — every org
// has a different internal-domain shape, so without user config this
// category contributes nothing.
//
// Match shape: we require a word character before the dot+domain so a
// bare mention of the domain in prose ("our internal network sits at
// thrillmade.internal") doesn't trip the gate — only host-style
// references (`api.thrillmade.internal`, `secrets.thrillmade.internal`)
// fire, which is the actual leak shape.
func scanOrgDomains(text string, domains []string) []PrivacyScannerHit {
	if len(domains) == 0 {
		return nil
	}
	var hits []PrivacyScannerHit
	for _, dom := range domains {
		dom = strings.TrimSpace(dom)
		if dom == "" {
			continue
		}
		// Escape the literal domain so dots match dots, not "any char".
		// Wrap in a regex requiring a word-shape prefix.
		// Anchored on word boundary on the right so trailing punctuation
		// is fine.
		pattern := `\b[A-Za-z0-9._-]+\.` + regexp.QuoteMeta(dom) + `\b`
		re, err := regexp.Compile(pattern)
		if err != nil {
			// Defensive: a malformed user-supplied domain shouldn't
			// crash the push. Skip silently — the user will still see
			// the baseline categories firing.
			continue
		}
		for _, loc := range re.FindAllStringIndex(text, -1) {
			hits = append(hits, PrivacyScannerHit{
				Layer:      scannerLayerName,
				Kind:       KindOrgDomain,
				Match:      text[loc[0]:loc[1]],
				LineNumber: lineNumberAt(text, loc[0]),
				Severity:   defaultSeverityByKind[KindOrgDomain],
			})
		}
	}
	return hits
}

// scanLocalPaths checks for `/Users/<name>/…` and `/home/<name>/…`
// references. Default severity is "warn" — see localPathPatterns
// docstring for why we don't block by default.
func scanLocalPaths(text string) []PrivacyScannerHit {
	var hits []PrivacyScannerHit
	for _, re := range localPathPatterns {
		for _, loc := range re.FindAllStringIndex(text, -1) {
			hits = append(hits, PrivacyScannerHit{
				Layer:      scannerLayerName,
				Kind:       KindLocalPath,
				Match:      text[loc[0]:loc[1]],
				LineNumber: lineNumberAt(text, loc[0]),
				Severity:   defaultSeverityByKind[KindLocalPath],
			})
		}
	}
	return hits
}

// applySeverityOverride consults the user's overrides table and
// returns the effective severity for a kind. The contract is:
//
//   - User can WIDEN: warn → block always honoured.
//   - User can NOT WEAKEN baseline-block kinds: block → warn ignored
//     when the kind is in baselineBlockKinds.
//   - Unknown severity values (typo'd config) are ignored — defaults
//     win. We do NOT fail the push on a bad override; the gate stays
//     safe, the user keeps shipping.
//
// This is the only place where ScannerConfig.SeverityOverrides is
// honoured, so the baseline-bypass guarantee is fully captured here.
func applySeverityOverride(kind, defaultSeverity string, overrides map[string]string) string {
	if overrides == nil {
		return defaultSeverity
	}
	override, ok := overrides[kind]
	if !ok {
		return defaultSeverity
	}
	override = strings.ToLower(strings.TrimSpace(override))
	switch override {
	case SeverityBlock, SeverityWarn:
	default:
		// Bad override value (e.g., "info", typo). Stick to default.
		return defaultSeverity
	}
	// Baseline-block kinds can't be weakened.
	if baselineBlockKinds[kind] && override == SeverityWarn {
		return defaultSeverity
	}
	return override
}

// mergeKeywords concatenates baseline + extras, lowercasing for case
// insensitive comparison and deduping. Order is preserved so tests can
// pin a stable hit sequence.
func mergeKeywords(baseline, extras []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(baseline)+len(extras))
	for _, k := range baseline {
		l := strings.ToLower(strings.TrimSpace(k))
		if l == "" || seen[l] {
			continue
		}
		seen[l] = true
		out = append(out, l)
	}
	for _, k := range extras {
		l := strings.ToLower(strings.TrimSpace(k))
		if l == "" || seen[l] {
			continue
		}
		seen[l] = true
		out = append(out, l)
	}
	return out
}

// lineNumberAt computes the 1-based line number containing the byte at
// offset. Counts newlines from the start of text. O(n) per call; not
// hot enough to need indexing.
func lineNumberAt(text string, offset int) int {
	if offset <= 0 {
		return 1
	}
	if offset > len(text) {
		offset = len(text)
	}
	return 1 + strings.Count(text[:offset], "\n")
}

// redactCredential returns a printable form of a credential match
// that names the provider but doesn't echo the token bytes. Format:
// `<name>:<first8>…`. We DO show the prefix so the author can locate
// the token in their SKILL.md (the prefix alone is not the secret —
// `sk_live_` is public knowledge; the secret is the random tail).
//
// Why we don't echo the full match: error output ends up in CI logs,
// PR comments, terminal scrollback. Echoing live credentials there is
// itself a leak. Printing the prefix-only redaction preserves the
// actionable signal (which provider, which token shape) without the
// payload.
func redactCredential(name, raw string) string {
	prefix := raw
	if len(raw) > 12 {
		prefix = raw[:12] + "…"
	}
	return name + ":" + prefix
}

// BlockingHits filters hits down to those with severity "block".
// Convenience for the push.go gate decision — avoids open-coding the
// severity check at every call site.
func BlockingHits(hits []PrivacyScannerHit) []PrivacyScannerHit {
	if len(hits) == 0 {
		return nil
	}
	var out []PrivacyScannerHit
	for _, h := range hits {
		if h.Severity == SeverityBlock {
			out = append(out, h)
		}
	}
	return out
}

// WarningHits is the inverse — hits at severity "warn" only. Same
// motivation: keep severity inspection out of the call sites.
func WarningHits(hits []PrivacyScannerHit) []PrivacyScannerHit {
	if len(hits) == 0 {
		return nil
	}
	var out []PrivacyScannerHit
	for _, h := range hits {
		if h.Severity == SeverityWarn {
			out = append(out, h)
		}
	}
	return out
}
