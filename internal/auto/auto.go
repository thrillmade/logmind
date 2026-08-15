// Package auto implements `logmind auto <profile>` — one-command setup
// for a repository that will be handed over to run unattended.
//
// What it does, and just as importantly what it does not:
//
//   - It writes the standing directive (`.logmind/auto.yml`) — the
//     durable policy the repo carries: what entry requires, the pause
//     threshold, where the checkpoint lives, the hard stops, and the
//     handback slots. NOT scheduler state; see the template's header.
//   - It REPORTS which required skills are present, and prints the
//     install command for any that are not. It does not fetch them:
//     seeding a repository from the catalog is the §5.2 subscription
//     model (skills-lock.json, origin + ref + content hash, arrive as a
//     PR), which is Planned and homed at skdd#6 — not something to
//     reimplement one repo at a time.
//   - It PRINTS the handover a human must give to start the mode. It
//     never starts it. `unattended-operation`'s own rule is that the
//     mode begins only from an explicit human handover and is never
//     inferred; a tool that started the mode would violate the policy it
//     just installed. This is also SPEC §2.5's "report; never
//     auto-apply" for anything nobody is watching.
//   - It never overwrites an existing directive. That file carries
//     policy a human authored — repo hard stops, the wake mechanism,
//     pre-authorized exceptions. Silently rewriting a declared hard stop
//     is precisely the failure this feature exists to prevent, so a
//     directive that is stale, newer, markerless, or written for another
//     profile is REPORTED and left alone.
//
// The cobra wiring is internal/cli/auto.go; the drift nudge is
// internal/doctor.collectAutoAdvisories. Both read through this package
// so "what is a profile" and "what does the marker mean" have one owner.
package auto

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/thrillmade/logmind/internal/inserter"
	"github.com/thrillmade/logmind/internal/skill"
	"github.com/thrillmade/logmind/internal/templates"
)

// Profile is one named setup recipe. Profiles are DATA — a new one is a
// template file plus an entry in the slice below, never a new branch in
// a switch. An unknown name is refused by Lookup and named against
// Names(); nothing falls back to a default.
type Profile struct {
	// Name is the argument the user types: `logmind auto <Name>`.
	Name string
	// Summary is the one-line description shown in --help and in the
	// unknown-profile refusal.
	Summary string
	// Mode is the prose name of what the profile sets up, used in the
	// printed handover ("logmind does NOT start <Mode>"). Carried as
	// data so the sentence cannot drift from the registry.
	Mode string
	// Skills are the SKILL.md directories this profile's policy depends
	// on, in load order (mechanism first, policy layered on it).
	Skills []string
	// Template is the bundled directive body's filename under
	// internal/templates/auto/.
	Template string
}

// profiles is the registry. Ordered, not a map, so --help and every
// refusal message list them the same way every run (SPEC §2.7: no
// ordering that depends on map iteration).
//
// Only `unattended` ships. Issue #241 also sketched `skdd` and `night`:
//
//   - `night` is deliberately absent. The skill it would install was
//     renamed night-mode → unattended-operation during review for a
//     reason this command inherits — the mode is triggered by a human
//     handover, never by the clock. A `logmind auto night` would teach
//     the vocabulary the policy rejects. See retiredProfiles.
//   - `skdd` has no content to write: nothing in the catalog or the
//     SPEC defines what an "skdd profile" declares that `logmind init`
//     does not already scaffold. It is refused like any other unknown
//     name until somebody says what it means.
var profiles = []Profile{
	{
		Name:    "unattended",
		Summary: "hand the session over to run unattended — heartbeat mechanism + unattended-operation policy",
		Mode:    "unattended operation",
		Skills:  []string{"session-heartbeat", "unattended-operation"},
		// Version marker inside: see templates.AutoDirective.
		Template: "unattended.yml.template",
	},
}

// retiredProfiles maps a name that once meant something to the note the
// refusal adds. It NEVER resolves to a profile — Lookup does not consult
// it — so this cannot become the silent fallback the registry exists to
// prevent. It only makes the refusal teach the current vocabulary.
var retiredProfiles = map[string]string{
	"night":      "`night-mode` was renamed `unattended-operation`: the mode is started by a human handover, never by the clock. Use `unattended`.",
	"night-mode": "`night-mode` was renamed `unattended-operation`: the mode is started by a human handover, never by the clock. Use `unattended`.",
}

// Profiles returns the registry in declaration order.
func Profiles() []Profile {
	out := make([]Profile, len(profiles))
	copy(out, profiles)
	return out
}

// Names returns every known profile name, in declaration order.
func Names() []string {
	out := make([]string, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, p.Name)
	}
	return out
}

// Lookup resolves a profile by exact name. The second return is false
// for ANY name the registry does not carry — including a retired one.
// There is no default, no prefix match, and no fallback: a caller that
// gets false must refuse and name Names().
func Lookup(name string) (Profile, bool) {
	for _, p := range profiles {
		if p.Name == name {
			return p, true
		}
	}
	return Profile{}, false
}

// RetiredNote returns the explanatory note for a name that used to mean
// something, and "" for a name that never did.
func RetiredNote(name string) string {
	return retiredProfiles[name]
}

// DirectivePath returns the standing directive's path: `.logmind/auto.yml`,
// alongside config.yml. Checked in — it is repository policy, not local
// state (contrast `.logmind/cache/` and `.logmind/.lock`, which the
// init-time .gitignore block excludes).
func DirectivePath(repoRoot string) string {
	return filepath.Join(repoRoot, ".logmind", "auto.yml")
}

var (
	// markerRe matches the SPEC §5.2 ownership marker on the directive's
	// first line — the same shape as the workflow templates'
	// `# logmind-template-version: vN` and the hooks'
	// `# logmind-hook-version: <ver>`.
	markerRe = regexp.MustCompile(`^# logmind-auto-version:\s*(\S+)`)
	// profileRe reads the directive's own `profile:` key. The profile
	// name is recorded ONCE, in the YAML body — putting it in the marker
	// comment too would be a second copy that reads as true until one
	// quietly isn't.
	profileRe = regexp.MustCompile(`(?m)^profile:[ \t]*(\S+)`)
)

// checkpointPlaceholder is substituted at render time, mirroring the
// workflow templates' __LOGMIND_VERSION__.
const checkpointPlaceholder = "__LOGMIND_CHECKPOINT__"

// CheckpointCandidates lists the conventional plan-doc paths probed for
// the checkpoint slot, in priority order. session-heartbeat requires
// "a durable file the project already reads — the plan doc", so the
// setup asks the repo what it already has rather than inventing a path.
// The first entry is also the fallback written when none exists (with
// the absence reported, never silently created — writing a plan doc's
// CONTENT is a judgment call, the same boundary `doctor --fix` draws for
// docs/spec.md).
var CheckpointCandidates = []string{"docs/plan.md", "PLAN.md", "plan.md"}

// ResolveCheckpoint picks the checkpoint path for repoRoot and reports
// whether that file exists yet.
func ResolveCheckpoint(repoRoot string) (relPath string, exists bool) {
	for _, c := range CheckpointCandidates {
		if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(c))); err == nil {
			return c, true
		}
	}
	return CheckpointCandidates[0], false
}

// BundledBody returns the rendered directive this binary would write for
// p against the given checkpoint path.
func BundledBody(p Profile, checkpoint string) string {
	return strings.ReplaceAll(templates.AutoDirective(p.Template), checkpointPlaceholder, checkpoint)
}

// BundledMarker returns the version marker embedded in p's template.
func BundledMarker(p Profile) (string, bool) {
	return parseMarker(templates.AutoDirective(p.Template))
}

func parseMarker(body string) (string, bool) {
	first := body
	if i := strings.Index(body, "\n"); i >= 0 {
		first = body[:i]
	}
	m := markerRe.FindStringSubmatch(first)
	if len(m) < 2 {
		return "", false
	}
	return m[1], true
}

func parseProfile(body string) (string, bool) {
	m := profileRe.FindStringSubmatch(body)
	if len(m) < 2 {
		return "", false
	}
	return m[1], true
}

// State is what `.logmind/auto.yml` looks like on disk.
type State struct {
	// Present is false when no directive is installed.
	Present bool
	// Marker is the installed `# logmind-auto-version:` value, "" when
	// the file carries none. A markerless artifact belongs to the user
	// and is never overwritten (SPEC §5.2).
	Marker string
	// Profile is the installed directive's `profile:` value, "" when
	// unreadable.
	Profile string
}

// Inspect reads the installed directive's markers without writing
// anything. A read error is indistinguishable from absence on purpose:
// every caller's response to "cannot read it" is the same as to "it is
// not there" — do not touch it.
func Inspect(repoRoot string) State {
	data, err := os.ReadFile(DirectivePath(repoRoot))
	if err != nil {
		return State{}
	}
	body := string(data)
	marker, _ := parseMarker(body)
	prof, _ := parseProfile(body)
	return State{Present: true, Marker: marker, Profile: prof}
}

// Outcome is what Apply did — one value, so a caller renders one line
// per case rather than reconstructing the decision from a bag of bools.
type Outcome string

const (
	// Created — no directive existed; one was written.
	Created Outcome = "created"
	// Current — the installed directive is this profile at this
	// binary's marker. Nothing written. This is the second-run case.
	Current Outcome = "current"
	// DeclinedStale — installed marker predates the bundled one. Left
	// alone: the file carries operator-authored policy.
	DeclinedStale Outcome = "declined-stale"
	// DeclinedNewer — installed marker is AHEAD of this binary's. Left
	// alone; upgrading logmind is the remedy (#286's direction rule).
	DeclinedNewer Outcome = "declined-newer"
	// DeclinedMarkerless — no ownership marker, so the file belongs to
	// the user (SPEC §5.2). Left alone.
	DeclinedMarkerless Outcome = "declined-markerless"
	// DeclinedOtherProfile — a directive for a different profile is
	// installed. Left alone; switching is a deliberate human act.
	DeclinedOtherProfile Outcome = "declined-other-profile"
)

// Result is Apply's full report.
type Result struct {
	Outcome Outcome
	// Path is the directive's path relative to repoRoot.
	Path string
	// Installed / Bundled are the two markers, for the declined cases.
	Installed string
	Bundled   string
	// InstalledProfile is the profile named by the file on disk
	// (DeclinedOtherProfile only).
	InstalledProfile string
	// Checkpoint is the resolved plan-doc path; CheckpointExists is
	// false when the repo has no plan doc yet.
	Checkpoint       string
	CheckpointExists bool
	// SkillsPresent / SkillsMissing partition Profile.Skills by whether
	// `.claude/skills/<name>/SKILL.md` is readable. Order follows the
	// profile's declared load order.
	SkillsPresent []string
	SkillsMissing []string
}

// Apply performs the setup for p against repoRoot and reports what it
// found. It writes at most one file — the directive — and only when
// none is installed. It starts nothing and runs no subprocess.
func Apply(repoRoot string, p Profile) (Result, error) {
	checkpoint, checkpointExists := ResolveCheckpoint(repoRoot)
	res := Result{
		Path:             filepath.ToSlash(filepath.Join(".logmind", "auto.yml")),
		Checkpoint:       checkpoint,
		CheckpointExists: checkpointExists,
	}
	bundled, _ := BundledMarker(p)
	res.Bundled = bundled

	for _, name := range p.Skills {
		if _, err := os.Stat(skill.MDPath(repoRoot, name)); err == nil {
			res.SkillsPresent = append(res.SkillsPresent, name)
			continue
		}
		res.SkillsMissing = append(res.SkillsMissing, name)
	}

	state := Inspect(repoRoot)
	switch {
	case !state.Present:
		body := BundledBody(p, checkpoint)
		target := DirectivePath(repoRoot)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return res, err
		}
		if err := os.WriteFile(target, []byte(body), 0o644); err != nil {
			return res, err
		}
		res.Outcome = Created
		res.Installed = bundled
		return res, nil
	case state.Marker == "":
		res.Outcome = DeclinedMarkerless
		return res, nil
	case state.Profile != p.Name:
		res.Outcome = DeclinedOtherProfile
		res.Installed = state.Marker
		res.InstalledProfile = state.Profile
		return res, nil
	case state.Marker == bundled:
		res.Outcome = Current
		res.Installed = state.Marker
		return res, nil
	}

	res.Installed = state.Marker
	if ahead(state.Marker, bundled) {
		res.Outcome = DeclinedNewer
		return res, nil
	}
	res.Outcome = DeclinedStale
	return res, nil
}

// ahead reports whether the installed marker orders AFTER the bundled
// one. Order, not inequality (#286): a released binary bundles older
// markers than dev, and a lexical compare gets "v11" vs "v4" exactly
// backwards. An unparseable marker on either side is NOT treated as
// ahead — the caller then reports it as stale, whose remedy (look at
// the file) is the safe direction.
//
// Delegates to inserter.ParseMarkerGeneration so the AGENTS.md block
// guard (#267), the workflow-template guard (#286) and this one order
// marker generations by one rule rather than three copies of it.
func ahead(installed, bundled string) bool {
	i, iok := inserter.ParseMarkerGeneration(installed)
	b, bok := inserter.ParseMarkerGeneration(bundled)
	return iok && bok && i > b
}

// InstallCommand returns the command an operator runs to install a
// missing skill from the catalog. Printed, never executed — logmind
// does not fetch catalog items (see the package comment).
//
// Shape matches the install line the AGENTS.md template already
// publishes for the `logmind` skill, so an operator sees one convention.
func InstallCommand(skillName string) string {
	return "npx skills add https://github.com/thrillmade/agent-skills --skill " + skillName
}
