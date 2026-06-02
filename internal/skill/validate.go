package skill

import (
	"fmt"
	"regexp"
	"strings"
)

// LogmindByteCap is the soft size cap (8 KB) Python applies via
// check_size_cap(). Tracks Python's _LOGMIND_SKILL_BYTE_CAP at v0.6.16
// — keep these two constants in lockstep when bumping.
const LogmindByteCap = 8000

// Anchored regexes mirror Python v0.6.0 PR #92's evidence-based fix:
// substring `"name:" in fm` false-positives on nested keys like
// `domain_name:` or `package_name:`. Multiline + start-anchor matches
// top-level fields AND fields nested under a single indent level (still
// real `name:` declarations) — same shape as Python's regexes.
var (
	frontmatterNameRE = regexp.MustCompile(`(?m)^\s*name\s*:`)
	frontmatterDescRE = regexp.MustCompile(`(?m)^\s*description\s*:`)
)

// CheckResult mirrors Python's (ok, message) tuple.
type CheckResult struct {
	OK      bool
	Message string
}

// CheckFrontmatter validates required-field presence in YAML
// frontmatter per the agentskills.io/v1 spec.
//
// Mirrors Python's check_frontmatter_required_fields. Error messages
// MUST be byte-identical to Python so the snapshot tests for `logmind
// skill test <name>` stay green.
//
// Lightweight check — proper validation is `skdd validate`'s job. This
// catches the most common authoring mistakes when skdd isn't available.
func CheckFrontmatter(content string) CheckResult {
	if !strings.HasPrefix(content, "---") {
		return CheckResult{false, "SKILL.md must start with YAML frontmatter (--- block)"}
	}
	// Python: end = content.find("\n---", 4); matches the closing
	// `\n---` AFTER the opening triple-dash + at least one body char.
	// Same offset preserves parity on documents where the body starts
	// immediately after the opening dashes.
	end := strings.Index(content[4:], "\n---")
	if end == -1 {
		return CheckResult{false, "SKILL.md frontmatter is unterminated (missing closing ---)"}
	}
	// fm = content[4:end] in Python (where end was the absolute index);
	// the Go Index call above is relative, so we add 4 back.
	fm := content[4 : 4+end]
	if !frontmatterNameRE.MatchString(fm) {
		return CheckResult{false, "SKILL.md frontmatter missing required field: name"}
	}
	if !frontmatterDescRE.MatchString(fm) {
		return CheckResult{false, "SKILL.md frontmatter missing required field: description"}
	}
	return CheckResult{true, ""}
}

// CheckSizeCap mirrors Python's check_size_cap. The size message is
// byte-identical to Python's f-string so `logmind skill test` snapshot
// tests don't drift.
func CheckSizeCap(content string, cap int) CheckResult {
	size := len(content) // Go strings are UTF-8 bytes; len() returns byte count.
	if size > cap {
		return CheckResult{
			OK: false,
			Message: fmt.Sprintf(
				"SKILL.md is %d bytes — over the %d-byte logmind cap. "+
					"Large skills bloat every agent load. Consider splitting "+
					"into multiple focused skills.",
				size, cap,
			),
		}
	}
	return CheckResult{
		OK:      true,
		Message: fmt.Sprintf("%d bytes (cap: %d)", size, cap),
	}
}
