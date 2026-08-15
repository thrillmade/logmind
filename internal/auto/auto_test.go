// auto_test.go — the profile registry's refusal contract, Apply's
// write/decline decisions, and the load-bearing check that the directive
// this package writes says what the two skills it installs say.
//
// The directive test is deliberately pinned on the RENDERED BODY (what an
// agent reads off disk), not on the template constant or on any helper
// here — a skill's rule surviving in a helper while the file loses it is
// exactly the failure this guards.
package auto

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}

func readDirective(t *testing.T, repoRoot string) string {
	t.Helper()
	data, err := os.ReadFile(DirectivePath(repoRoot))
	if err != nil {
		t.Fatalf("read directive: %v", err)
	}
	return string(data)
}

// TestLookup_UnknownNameNeverResolves is the anti-fallback guard: an
// unknown profile must not silently become a default. Every name below is
// outside the registry — including one that used to mean something, one
// the issue sketched but nobody defined, and a case/prefix variant of a
// real name.
func TestLookup_UnknownNameNeverResolves(t *testing.T) {
	for _, name := range []string{"", "night", "night-mode", "skdd", "UNATTENDED", "unatt", "unattended-operation"} {
		if p, ok := Lookup(name); ok {
			t.Errorf("Lookup(%q) resolved to %q; an unknown profile must be refused, never defaulted", name, p.Name)
		}
	}
}

func TestLookup_KnownNameResolvesExactly(t *testing.T) {
	p, ok := Lookup("unattended")
	if !ok {
		t.Fatalf("Lookup(%q) = false; want the registered profile", "unattended")
	}
	if p.Name != "unattended" {
		t.Errorf("Name = %q; want unattended", p.Name)
	}
	if got, want := strings.Join(p.Skills, ","), "session-heartbeat,unattended-operation"; got != want {
		t.Errorf("Skills = %q; want %q (mechanism first, policy layered on it)", got, want)
	}
}

// TestNames_ListsEveryProfileInOrder pins the refusal's vocabulary source:
// whatever Lookup accepts, Names() must be able to name.
func TestNames_ListsEveryProfileInOrder(t *testing.T) {
	names := Names()
	if len(names) != len(Profiles()) {
		t.Fatalf("Names() = %v (%d); want one per profile (%d)", names, len(names), len(Profiles()))
	}
	for i, p := range Profiles() {
		if names[i] != p.Name {
			t.Errorf("Names()[%d] = %q; want %q (declaration order)", i, names[i], p.Name)
		}
		if _, ok := Lookup(p.Name); !ok {
			t.Errorf("Lookup(%q) = false for a name the registry lists", p.Name)
		}
	}
}

// TestRetiredNote_NeverResolvesToAProfile — the didactic note for a
// retired name must stay a MESSAGE. If it ever became a lookup path it
// would be the silent fallback the registry exists to prevent.
func TestRetiredNote_NeverResolvesToAProfile(t *testing.T) {
	if note := RetiredNote("night"); note == "" {
		t.Errorf("RetiredNote(%q) = \"\"; want the rename explanation", "night")
	}
	if _, ok := Lookup("night"); ok {
		t.Errorf("Lookup(%q) resolved; a retired name is explained, never honoured", "night")
	}
	if note := RetiredNote("unattended"); note != "" {
		t.Errorf("RetiredNote(%q) = %q; a live profile is not retired", "unattended", note)
	}
}

// TestEveryProfileTemplateIsSelfConsistent — the template's own
// `profile:` key must equal the registry name, and its first line must
// carry the SPEC §5.2 ownership marker. Without both, Inspect classifies a
// freshly written directive as belonging to somebody else.
func TestEveryProfileTemplateIsSelfConsistent(t *testing.T) {
	for _, p := range Profiles() {
		body := BundledBody(p, "docs/plan.md")
		marker, ok := BundledMarker(p)
		if !ok || marker == "" {
			t.Errorf("profile %q: no `# logmind-auto-version:` marker on the first line", p.Name)
		}
		named, ok := parseProfile(body)
		if !ok || named != p.Name {
			t.Errorf("profile %q: template declares profile: %q", p.Name, named)
		}
		if p.Mode == "" || p.Summary == "" {
			t.Errorf("profile %q: Mode/Summary must be set — the CLI prints both", p.Name)
		}
	}
}

// TestDirective_SaysWhatTheSkillsSay is the agreement contract. Each case
// names the skill that owns the rule and a phrase the directive must
// carry; if a skill changes and the template does not, this fails.
//
// Sources:
//
//	session-heartbeat     — beat mechanism, threshold, checkpoint slots
//	unattended-operation  — handover contract, authority, hard stops, digest
func TestDirective_SaysWhatTheSkillsSay(t *testing.T) {
	p, _ := Lookup("unattended")
	body := BundledBody(p, "docs/plan.md")

	required := []struct {
		rule  string // why the phrase has to be there
		want  string
		skill string
	}{
		// unattended-operation: "Entry is a handover contract" — the mode
		// begins from a human handover and is never inferred.
		{"entry requires a human handover", "requires_human_handover: true", "unattended-operation"},
		{"handover names the scope", "scope — the named plan items or slice range", "unattended-operation"},
		{"handover names the hard stops", "hard stops — what ends a lane", "unattended-operation"},
		{"handover names exceptions BY NAME", "pre-authorized exceptions, by name", "unattended-operation"},
		{"handover names the wake mechanism", "the wake mechanism, and where parked work is written", "unattended-operation"},
		{"handover says what to do at a fork", "at a real fork", "unattended-operation"},

		// session-heartbeat: the threshold is DERIVED, not a round number.
		{"threshold is derived from the largest dispatch", "the ceiling, minus the cost of the largest dispatch you would still start", "session-heartbeat"},
		{"threshold is enforceable per dispatch", "if this agent cannot finish inside the remaining headroom, do not start it", "session-heartbeat"},
		{"at the threshold, do not kill what is running", "do NOT kill what is running", "session-heartbeat"},
		{"wake for the reset, not a retry", "schedule the wake for the reset, not a short retry", "session-heartbeat"},

		// session-heartbeat: the checkpoint's required slots.
		{"checkpoint records a sha, not a branch", "a sha, never a branch name", "session-heartbeat"},
		{"checkpoint records what is in flight", "in flight — per agent", "session-heartbeat"},
		{"checkpoint records occupancy", "occupancy — which trees are not free to merge into", "session-heartbeat"},
		{"checkpoint pre-decides the next dispatch", "next dispatch, already decided", "session-heartbeat"},
		{"checkpoint distinguishes paused from died", "next wake time, and why", "session-heartbeat"},

		// unattended-operation: the authority test and its two sides.
		{"the reversible-and-invisible test", "can it be undone before they are back", "unattended-operation"},
		{"local commits are the near side", "local commits on local branches", "unattended-operation"},
		{"pushing a shared ref is the far side", "pushing to a shared ref", "unattended-operation"},
		{"anything sent is the far side", "anything sent — mail, message, webhook, notification", "unattended-operation"},
		{"--force/--no-verify/--admin is the far side", "--force, --no-verify, or --admin", "unattended-operation"},

		// unattended-operation: a wake is never consent.
		{"a scheduled wake is not consent", "a scheduled wake, a task notification", "unattended-operation"},
		{"a green check is not consent", "a passing suite, a green check, or a clean review", "unattended-operation"},
		{"another agent's approval is not consent", `another agent reporting "done", "approved"`, "unattended-operation"},

		// unattended-operation: hard stops that hold unnamed.
		{"a scope change is a standing hard stop", "a change of scope", "unattended-operation"},
		{"a destructive operation is a standing hard stop", "a destructive operation", "unattended-operation"},
		{"a human-required gate is a standing hard stop", "a gate that requires a human", "unattended-operation"},
		{"a second consecutive failure is a standing hard stop", "the second consecutive failure of the same fix", "unattended-operation"},

		// unattended-operation: the six handback slots, outcome first.
		{"digest slot 1", "where the work stands now — one line", "unattended-operation"},
		{"digest slot 2", "landed — sha plus one line each", "unattended-operation"},
		{"digest slot 3", "including what was found and not fixed", "unattended-operation"},
		{"digest slot 4", `never "needs input"`, "unattended-operation"},
		{"digest slot 5", "not attempted — in scope, skipped, and why", "unattended-operation"},
		{"digest slot 6", "standing — budget/limit state", "unattended-operation"},

		// unattended-operation, silent failures: scheduler state committed.
		{"scheduler state must be git-ignored", "must_be_git_ignored: true", "unattended-operation"},
	}
	for _, c := range required {
		if !strings.Contains(body, c.want) {
			t.Errorf("directive is missing %s (%s owns this rule); expected to find %q",
				c.rule, c.skill, c.want)
		}
	}

	// Both skills are named as sources, and both are declared as required
	// loads — a directive that restates a rule without naming its owner
	// goes stale invisibly.
	for _, name := range p.Skills {
		if !strings.Contains(body, "  - "+name) {
			t.Errorf("directive does not list %q under skills:", name)
		}
		if !strings.Contains(body, "skills/"+name) {
			t.Errorf("directive does not link the %q skill as the rule's owner", name)
		}
	}
}

// TestDirective_DoesNotHardcodeAPercentageThreshold — the issue sketched
// "90% of session window"; session-heartbeat explicitly refutes that
// ("Stopping at 90% used only works if nothing you would start costs more
// than the remaining 10%"). The directive must carry the derivation.
func TestDirective_DoesNotHardcodeAPercentageThreshold(t *testing.T) {
	p, _ := Lookup("unattended")
	body := BundledBody(p, "docs/plan.md")
	for _, bad := range []string{"90%", "0.9", "90 percent"} {
		if strings.Contains(body, bad) {
			t.Errorf("directive hardcodes %q as the pause threshold; session-heartbeat derives it from the largest dispatch", bad)
		}
	}
}

// TestDirective_IsPolicyNotSchedulerState — ruling from
// unattended-operation's silent-failure table: the wake mechanism's live
// state must never land as a project artifact. The directive declares
// WHERE running state goes (the checkpoint path); it never carries the
// state itself.
func TestDirective_IsPolicyNotSchedulerState(t *testing.T) {
	p, _ := Lookup("unattended")
	body := BundledBody(p, "docs/plan.md")
	for _, bad := range []string{"next_wake:", "next_wake_at", "in_flight:", "current_sha", "last_beat", "run_started"} {
		if strings.Contains(body, bad) {
			t.Errorf("directive carries live scheduler state (%q); it must hold durable policy only", bad)
		}
	}
	if !strings.Contains(body, "path: docs/plan.md") {
		t.Errorf("directive does not name where running state goes (checkpoint path)")
	}
}

func TestResolveCheckpoint_PrefersWhatTheRepoAlreadyHas(t *testing.T) {
	t.Run("no plan doc falls back to the first candidate and reports absence", func(t *testing.T) {
		dir := t.TempDir()
		path, exists := ResolveCheckpoint(dir)
		if path != CheckpointCandidates[0] || exists {
			t.Errorf("ResolveCheckpoint = (%q, %v); want (%q, false)", path, exists, CheckpointCandidates[0])
		}
	})
	t.Run("a lower-priority candidate is used when it is the one present", func(t *testing.T) {
		dir := t.TempDir()
		mustWrite(t, filepath.Join(dir, "PLAN.md"), "# Plan\n")
		path, exists := ResolveCheckpoint(dir)
		if path != "PLAN.md" || !exists {
			t.Errorf("ResolveCheckpoint = (%q, %v); want (PLAN.md, true)", path, exists)
		}
	})
}

func TestApply_CreatesDirectiveThenLeavesItAlone(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "docs", "plan.md"), "# Plan\n")
	p, _ := Lookup("unattended")

	first, err := Apply(dir, p)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if first.Outcome != Created {
		t.Fatalf("first Apply outcome = %q; want %q", first.Outcome, Created)
	}
	body1 := readDirective(t, dir)
	if !strings.Contains(body1, "path: docs/plan.md") {
		t.Errorf("checkpoint placeholder not rendered:\n%s", body1)
	}
	if strings.Contains(body1, "__LOGMIND_CHECKPOINT__") {
		t.Errorf("placeholder survived into the written directive")
	}

	second, err := Apply(dir, p)
	if err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if second.Outcome != Current {
		t.Errorf("second Apply outcome = %q; want %q", second.Outcome, Current)
	}
	if body2 := readDirective(t, dir); body2 != body1 {
		t.Errorf("second Apply rewrote the directive; it must be a no-op")
	}
}

// TestApply_NeverOverwritesADirectiveItDoesNotOwn covers all four
// decline paths. Each one leaves the bytes on disk EXACTLY as found —
// the file carries policy a human authored.
func TestApply_NeverOverwritesADirectiveItDoesNotOwn(t *testing.T) {
	p, _ := Lookup("unattended")
	bundled, _ := BundledMarker(p)

	cases := []struct {
		name    string
		content string
		want    Outcome
	}{
		{
			name:    "markerless belongs to the user",
			content: "profile: unattended\nhard_stops:\n  repo: [never push]\n",
			want:    DeclinedMarkerless,
		},
		{
			name:    "a directive for another profile",
			content: "# logmind-auto-version: " + bundled + "\nprofile: skdd\n",
			want:    DeclinedOtherProfile,
		},
		{
			name:    "an older marker",
			content: "# logmind-auto-version: v0\nprofile: unattended\n",
			want:    DeclinedStale,
		},
		{
			name:    "a newer marker is not downgraded",
			content: "# logmind-auto-version: v99\nprofile: unattended\n",
			want:    DeclinedNewer,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			mustWrite(t, DirectivePath(dir), c.content)
			res, err := Apply(dir, p)
			if err != nil {
				t.Fatalf("Apply: %v", err)
			}
			if res.Outcome != c.want {
				t.Errorf("outcome = %q; want %q", res.Outcome, c.want)
			}
			if got := readDirective(t, dir); got != c.content {
				t.Errorf("Apply rewrote a directive it does not own:\n got: %q\nwant: %q", got, c.content)
			}
		})
	}
}

func TestApply_PartitionsSkillsByPresence(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, ".claude", "skills", "session-heartbeat", "SKILL.md"), "---\nname: session-heartbeat\n---\n")
	p, _ := Lookup("unattended")

	res, err := Apply(dir, p)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got, want := strings.Join(res.SkillsPresent, ","), "session-heartbeat"; got != want {
		t.Errorf("SkillsPresent = %q; want %q", got, want)
	}
	if got, want := strings.Join(res.SkillsMissing, ","), "unattended-operation"; got != want {
		t.Errorf("SkillsMissing = %q; want %q", got, want)
	}
}

// TestApply_DoesNotFetchSkills — logmind never pulls catalog items (that
// is the §5.2 subscription model, Planned at skdd#6). A missing skill is
// reported with the command a human runs, and the skill directory stays
// absent.
func TestApply_DoesNotFetchSkills(t *testing.T) {
	dir := t.TempDir()
	p, _ := Lookup("unattended")
	if _, err := Apply(dir, p); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	for _, name := range p.Skills {
		if _, err := os.Stat(filepath.Join(dir, ".claude", "skills", name)); err == nil {
			t.Errorf("Apply materialised skill %q; it must only report the install command", name)
		}
	}
	if cmd := InstallCommand("session-heartbeat"); !strings.Contains(cmd, "session-heartbeat") ||
		!strings.Contains(cmd, "thrillmade/agent-skills") {
		t.Errorf("InstallCommand = %q; want it to name the skill and the catalog", cmd)
	}
}
