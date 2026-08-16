import { CopyButton } from "./copy-button";

const BREW = "brew install thrillmade/tap/logmind";
const CURL = "curl -fsSL https://logmind.dev/install.sh | sh";
const SKILL = "npx skills add -g thrillmade/logmind-skill";
const VERIFY = "logmind --version";
// Frozen at v0.6.16 — the last published Python release. Pinned only so
// consumer repos that still hardcode `logmind==0.6.x` keep resolving.
const PIP_LEGACY = "pip install 'logmind==0.6.16'";

// ---------------------------------------------------------------------------
// Version truth — single source for every version string this page shows.
//
// TO RELEASE v2.0.0: update these three lines (CURRENT_VERSION,
// CURRENT_SPEC, CURRENT_RELEASE_DATE) — everything below derives from
// them. The hero badge's "vX design in progress" caveat clears itself
// automatically once CURRENT_VERSION === NEXT_VERSION: NEXT_VERSION only
// ever names "whatever comes after CURRENT_VERSION", so that comparison
// is the right one for it.
//
// Every feature-specific caveat is different: it clears when
// CURRENT_VERSION reaches the release that ACTUALLY introduced that
// feature, which is a fixed fact about the past, not "whatever
// NEXT_VERSION currently says". IS_NEXT_RELEASED (CURRENT_VERSION ===
// NEXT_VERSION) is wrong for these — it goes false again the moment the
// next dev cycle opens and NEXT_VERSION moves on, even though the
// feature that shipped at the earlier release is still shipped. AREAS_SINCE
// (the areas line) and ENFORCEMENT_SINCE (the commit-guard hooks) are
// both this shape: each is gated on "CURRENT_VERSION >= <ITS_OWN>_SINCE"
// via compareVersions, and the version NAMED in that section's prose is
// the same `*_SINCE` constant too — never NEXT_VERSION, which would name
// the wrong release the moment it moves past the one that actually
// shipped the feature. `*_SINCE` constants only need touching if the
// release that introduced their feature is ever revised (it should not
// be, once true) — not at every version bump, and not in sync with
// NEXT_VERSION.
//
// No other edit is required at release time — UNLESS
// internal/version/version.go's own `Areas` string has changed since
// AREAS below was last copied. AREAS is not part of the three-line flip:
// it mirrors what the binary claims under SPEC §7.3, which is orthogonal
// to the version number, so it only needs touching when the binary's
// declared areas change, not at every release.
//
// CURRENT_* is what `brew install thrillmade/tap/logmind` / curl / the
// skill installs *today*. Verified against `gh release list --repo
// thrillmade/logmind` (latest tag) and the installed binary's own
// `logmind --version` output — re-verify both at every release.
//
// NEXT_VERSION names the release the hero badge and the install
// footnote describe ahead of time, generically — "there's a next
// release and it isn't out yet" — never a specific feature's own
// availability; see the `*_SINCE` constants above for those. The site
// can't import internal/version/version.go (Go vs TS), so this block is
// a hand-maintained mirror of it — keep them in sync at tag time.
// ---------------------------------------------------------------------------
// Typed `string` (not narrowed to a literal) so the equality check below
// type-checks at every point in the release cycle, including the moment
// CURRENT_VERSION becomes "2.0.0" and this stops being a static "always
// false" comparison.
const CURRENT_VERSION: string = "1.2.0";
const CURRENT_SPEC = "0.1.1";
const CURRENT_RELEASE_DATE = "2026-06-07";
const NEXT_VERSION: string = "2.0.0";
const IS_NEXT_RELEASED = CURRENT_VERSION === NEXT_VERSION;
// SPEC §7.3's second `--version` line — mirrored byte-for-byte from
// internal/version/version.go's `Areas` (comma-and-space-joined, fixed
// vocabulary order). AREAS_SINCE names the release whose binary first
// prints this line (it landed alongside the v2.0.0 cut). Gated on
// "CURRENT_VERSION >= AREAS_SINCE", NOT on IS_NEXT_RELEASED
// (CURRENT_VERSION === NEXT_VERSION): those two questions only coincide
// while NEXT_VERSION happens to equal AREAS_SINCE, today. The moment the
// v2.0.0 tag lands and the next dev cycle opens NEXT_VERSION on to
// "2.1.0", IS_NEXT_RELEASED goes permanently false even though the
// *released* 2.0.0 binary keeps printing the areas line — using it here
// would silently drop the line from the site the day after the release
// it was added to celebrate. compareVersions compares numerically, not
// as strings: "2.10.0" < "2.9.0" lexically, and that is a real future
// version number.
const AREAS_SINCE = "2.0.0";
const AREAS = "orient, work, record, propagate, gates";

// The "enforced" section's two-layer commit guard (commit-msg hook +
// Claude Code PreToolUse hook) shipped in v2.0.0 — see
// internal/version/version.go's SameMajor doc comment: "the enforcement
// gate itself arrived in 2.0.0". Same shape as AREAS_SINCE above, and
// for the same reason: the guard is a fact about the release that
// introduced it, not about whatever NEXT_VERSION currently is. Both the
// "enforced" section's caveat AND the version it names in its own prose
// (v{ENFORCEMENT_SINCE}, not v{NEXT_VERSION}) key off this constant —
// naming NEXT_VERSION there would claim the wrong release added the
// guard the moment NEXT_VERSION moves past v2.0.0.
const ENFORCEMENT_SINCE = "2.0.0";

// compareVersions compares two plain "major.minor.patch" version strings
// numerically. CURRENT_VERSION / NEXT_VERSION / AREAS_SINCE /
// ENFORCEMENT_SINCE on this page are always plain release numbers — no
// "-dev"/prerelease suffix; that only exists on the Go binary's own
// unreleased builds — so a bare split-on-"." + Number compare is
// sufficient here.
function compareVersions(a: string, b: string): number {
  const pa = a.split(".").map(Number);
  const pb = b.split(".").map(Number);
  for (let i = 0; i < 3; i++) {
    const diff = (pa[i] ?? 0) - (pb[i] ?? 0);
    if (diff !== 0) return diff;
  }
  return 0;
}

const SHOWS_AREAS_LINE = compareVersions(CURRENT_VERSION, AREAS_SINCE) >= 0;
const SHOWS_ENFORCEMENT_AVAILABLE = compareVersions(CURRENT_VERSION, ENFORCEMENT_SINCE) >= 0;
const VERIFY_OUTPUT = SHOWS_AREAS_LINE
  ? `logmind ${CURRENT_VERSION} (spec ${CURRENT_SPEC})\nareas: ${AREAS}`
  : `logmind ${CURRENT_VERSION} (spec ${CURRENT_SPEC})`;

const QUICKSTART = `$ logmind init
$ git checkout -b feat/auth
$ logmind log "JWT for stateless API auth" \\
    -r "horizontal scaling without session store" \\
    -a "server sessions in Redis" \\
    -i "rotate signing keys quarterly" \\
    -H "Added JWT session auth with refresh-token rotation"
✓ wrote docs/decisions-branches/feat__auth.md
✓ headline set — surfaces at the top of docs/timeline.md`;

function CommandBlock({
  cmd,
  hint,
  index,
}: {
  cmd: string;
  hint: string;
  index: string;
}) {
  return (
    <div className="grid grid-cols-[3rem_1fr] sm:grid-cols-[5rem_1fr] gap-x-4 sm:gap-x-6 items-start py-5 border-t border-rule">
      <div className="marginalia pt-1 text-right">
        {index}
        <br />
        <span className="text-foreground/70 normal-case tracking-normal">{hint}</span>
      </div>
      <div className="font-mono text-sm sm:text-[15px] flex items-center justify-between gap-4 group">
        <code className="text-foreground/95 break-all">
          <span className="text-accent select-none">$ </span>
          {cmd}
        </code>
        <CopyButton text={cmd} label={`Copy ${hint} command`} />
      </div>
    </div>
  );
}

function NavLink({ href, children }: { href: string; children: React.ReactNode }) {
  return (
    <a
      href={href}
      target="_blank"
      rel="noopener noreferrer"
      className="font-mono text-xs uppercase tracking-[0.18em] text-muted hover:text-foreground transition-colors"
    >
      {children}
    </a>
  );
}

export default function Home() {
  return (
    <div className="relative z-10 flex flex-col min-h-screen">
      {/* nav */}
      <header className="px-6 sm:px-10 lg:px-16 py-6">
        <div className="max-w-6xl mx-auto w-full flex items-center justify-between">
          <a href="/" className="display text-xl tracking-tight">
            logmind<span className="text-accent">.</span>
          </a>
          <nav className="flex items-center gap-6 sm:gap-8">
            <NavLink href="https://github.com/thrillmade/logmind">github</NavLink>
            <NavLink href="https://github.com/thrillmade/logmind/releases/latest">releases</NavLink>
            <NavLink href="https://skills.sh/thrillmade/logmind-skill">skill</NavLink>
          </nav>
        </div>
      </header>

      {/* hero */}
      <section className="px-6 sm:px-10 lg:px-16 pt-12 sm:pt-24 pb-20">
        <div className="max-w-6xl mx-auto w-full">
          <div className="rise marginalia mb-6">
            v{CURRENT_VERSION} ⁄ released {CURRENT_RELEASE_DATE} ⁄ MIT{" "}
            {!IS_NEXT_RELEASED && `⁄ v${NEXT_VERSION} design in progress `}
            ⁄ <a href="https://zakelfassi.com/skdd-skills-driven-development" className="hover:text-accent transition-colors">a substrate for SkDD</a>
          </div>
          <h1 className="rise display text-[12vw] sm:text-[8.5vw] leading-[0.92] font-light max-w-[16ch]" style={{ animationDelay: "0.05s" }}>
            Infinite context
            <span className="text-accent">.</span>
            <br />
            <span className="italic font-normal text-muted">For every agent</span>
            <br />
            <span className="text-muted">you ever hire.</span>
          </h1>

          <div className="rise mt-12 grid grid-cols-1 sm:grid-cols-12 gap-8 sm:gap-12" style={{ animationDelay: "0.18s" }}>
            <p className="lead sm:col-span-8 text-lg leading-[1.55] text-foreground/85">
              <span className="text-accent">Capture decisions while you work.</span> One CLI
              command per choice — no after-the-fact writeup. Every new
              agent (Claude Code, Cursor, Codex, Cline) gets the full
              <em> why-behind-the-code </em>in one read. Markdown links between
              docs are checked on every PR; the project tree regenerates on
              every log. Documentation that can&apos;t go stale.
            </p>
            <aside className="sm:col-span-4 sm:border-l sm:border-rule sm:pl-6 marginalia space-y-3 self-start pt-2">
              <div>
                <span className="block">made for</span>
                <span className="block text-foreground/85 normal-case tracking-normal">
                  teams shipping with AI agents
                </span>
              </div>
              <div>
                <span className="block">replaces</span>
                <span className="block text-foreground/85 normal-case tracking-normal">
                  decisions that lived in someone&apos;s head
                </span>
              </div>
              <div>
                <span className="block">onboards</span>
                <span className="block text-foreground/85 normal-case tracking-normal">
                  new agents in one read
                </span>
              </div>
            </aside>
          </div>

          <div className="rise mt-12 flex flex-col sm:flex-row sm:items-center gap-3 sm:gap-4 text-sm" style={{ animationDelay: "0.32s" }}>
            <a
              href="https://github.com/thrillmade/logmind"
              target="_blank"
              rel="noopener noreferrer"
              className="font-mono uppercase tracking-[0.18em] px-5 py-3 bg-paper text-background hover:bg-accent hover:text-paper transition-colors text-center whitespace-nowrap"
            >
              read the source →
            </a>
            <a
              href="#install"
              className="font-mono uppercase tracking-[0.18em] px-5 py-3 border border-rule hover:border-accent hover:text-accent transition-colors text-center whitespace-nowrap"
            >
              install ↓
            </a>
          </div>
        </div>
      </section>

      {/* what — three principles */}
      <section className="px-6 sm:px-10 lg:px-16 py-20 border-t border-rule">
        <div className="max-w-6xl mx-auto w-full">
          <h2 className="display text-4xl sm:text-5xl font-light leading-tight mb-12">
            principles<span className="text-accent">.</span>
          </h2>

          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-x-10 gap-y-12">
            {[
              {
                n: "i.",
                title: "captured while you work",
                body: (
                  <>
                    One{" "}
                    <code className="font-mono text-foreground text-[0.85em]">
                      logmind log
                    </code>{" "}
                    per choice — written, committed, and routed to the right
                    branch file in one command. Reasoning lives next to the
                    code, in git.
                  </>
                ),
              },
              {
                n: "ii.",
                title: "auto-documentation",
                body: (
                  <>
                    AGENTS.md is canonical; per-tool files become two-line
                    stubs pointing at it. Every relative{" "}
                    <code className="font-mono text-foreground text-[0.85em]">
                      [link](path.md)
                    </code>{" "}
                    is verified on every PR. The project tree regenerates on
                    every log.
                  </>
                ),
              },
              {
                n: "iii.",
                title: "always in sync",
                body: (
                  <>
                    Decisions, timeline, project tree —{" "}
                    <em>they update together</em>. Every branch also carries
                    a one-sentence, agent-authored headline that surfaces at
                    the top of the main-canonical timeline the moment a PR
                    opens. Rebase against main? Fresh clone? Multi-commit
                    amend? They stay synced. CI never catches you with a
                    stale derived doc.
                  </>
                ),
              },
              {
                n: "iv.",
                title: "infinite context for agents",
                body: (
                  <>
                    Any agent that supports{" "}
                    <code className="font-mono text-foreground text-[0.85em]">
                      AGENTS.md
                    </code>{" "}
                    inherits the decision history, the tree, the
                    why-behind-the-code — one{" "}
                    <code className="font-mono text-foreground text-[0.85em]">
                      logmind context
                    </code>{" "}
                    call away. Add the repomap (Go + TS/JS signature
                    skeletons, ranked and token-budgeted) and an agent sees
                    the whole codebase&apos;s shape before opening a single
                    file. Onboarding a new agent takes <em>one read</em>.
                  </>
                ),
              },
            ].map((p) => (
              <div key={p.n} className="grid grid-cols-[1.6rem_1fr] gap-3">
                <div className="display text-accent font-light text-base pt-1">
                  {p.n}
                </div>
                <div>
                  <h3 className="display text-2xl mb-3 italic font-normal">
                    {p.title}
                  </h3>
                  <p className="text-[15px] leading-[1.65] text-foreground/75">
                    {p.body}
                  </p>
                </div>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* pairs with clud-bug — the end-to-end SkDD loop */}
      <section className="px-6 sm:px-10 lg:px-16 py-20 border-t border-rule">
        <div className="max-w-6xl mx-auto w-full grid grid-cols-1 sm:grid-cols-12 gap-8">
          <div className="sm:col-span-4">
            <h2 className="display text-4xl sm:text-5xl font-light leading-tight">
              the loop<span className="text-accent">.</span>
            </h2>
            <p className="text-[15px] mt-4 text-foreground/70 leading-relaxed max-w-xs">
              Logmind is one part of an end-to-end agentic dev loop. The
              tools compose; each can stand alone.
            </p>
          </div>
          <div className="sm:col-span-8">
            <p className="text-[15px] leading-[1.65] text-foreground/85 mb-6">
              <a href="https://zakelfassi.com/skdd-skills-driven-development" className="text-accent hover:underline">Skills-driven development</a> (SkDD) gives you a methodology and
              skill primitives. <strong>logmind</strong> captures the <em>why</em> behind every
              change as you work, builds and tests the skills you forge,
              and keeps your derived docs in sync across rebase / merge /
              fresh clone. <a href="https://cludbug.dev" className="text-accent hover:underline">clud-bug</a> runs your skills against every PR — every
              finding cites the skill that motivated it. The loop closes
              when logmind logs the review outcome and surfaces refinement
              patterns for the next iteration.
            </p>
            <p className="marginalia normal-case tracking-normal text-foreground/55 mt-4 text-xs leading-relaxed">
              End-to-end agentic auto dev: write skills first → log the
              why → run them against PRs → iterate based on usage.
              The tools work independently; better together.
            </p>
          </div>
        </div>
      </section>

      {/* enforced — the two-layer commit guard */}
      <section className="px-6 sm:px-10 lg:px-16 py-20 border-t border-rule">
        <div className="max-w-6xl mx-auto w-full grid grid-cols-1 sm:grid-cols-12 gap-8">
          <div className="sm:col-span-4">
            <h2 className="display text-4xl sm:text-5xl font-light leading-tight">
              enforced<span className="text-accent">.</span>
            </h2>
            <p className="text-[15px] mt-4 text-foreground/70 leading-relaxed max-w-xs">
              A convention only holds if the tooling holds the door. v{ENFORCEMENT_SINCE}{" "}
              adds a guard in front of every substantive commit
              {!SHOWS_ENFORCEMENT_AVAILABLE && " — in design, not yet released"}.
            </p>
          </div>
          <div className="sm:col-span-8">
            <p className="text-[15px] leading-[1.65] text-foreground/85 mb-6">
              {!SHOWS_ENFORCEMENT_AVAILABLE && (
                <>
                  <strong className="text-accent">Not yet released</strong> —
                  ships with v{ENFORCEMENT_SINCE}.{" "}
                </>
              )}
              A <code className="font-mono text-foreground text-[0.85em]">commit-msg</code> hook and a Claude
              Code <code className="font-mono text-foreground text-[0.85em]">PreToolUse</code> hook check the
              same rule: a substantive change with no matching{" "}
              <code className="font-mono text-foreground text-[0.85em]">logmind log</code> entry gets blocked,
              not just flagged after the fact. Escape hatches exist for the
              commits that genuinely aren&apos;t decisions —{" "}
              <code className="font-mono text-foreground text-[0.85em]">[skip-logmind]</code> in the subject,{" "}
              <code className="font-mono text-foreground text-[0.85em]">LOGMIND_ALLOW_GIT_COMMIT=1</code> for
              one command, <code className="font-mono text-foreground text-[0.85em]">git.enforce_commits: false</code>{" "}
              to opt a repo out entirely. The git hook fails open by
              design: anything but the guard&apos;s explicit block signal —
              a stale, missing, or erroring logmind binary included —
              lets the commit through —
              enforcement can&apos;t become an outage.
            </p>
            <p className="marginalia normal-case tracking-normal text-foreground/55 mt-4 text-xs leading-relaxed">
              The same orientation-first discipline covers the spec:{" "}
              <code className="font-mono">docs/spec.md</code> (or any path
              via <code className="font-mono">context.spec_file</code>) is a
              hand-authored, forward-looking <strong>WHERE-TO</strong> —
              alongside the derived <strong>WHY</strong> (
              <code className="font-mono">docs/timeline.md</code>) and{" "}
              <strong>WHAT</strong> (
              <code className="font-mono">docs/file-structure.md</code> /{" "}
              <code className="font-mono">logmind repomap</code>). Configured,
              it folds into <code className="font-mono">logmind context</code>{" "}
              as the first document — the most stable, so the best cache
              prefix.
            </p>
          </div>
        </div>
      </section>

      {/* measured */}
      <section className="px-6 sm:px-10 lg:px-16 py-20 border-t border-rule">
        <div className="max-w-6xl mx-auto w-full grid grid-cols-1 sm:grid-cols-12 gap-8">
          <div className="sm:col-span-4">
            <h2 className="display text-4xl sm:text-5xl font-light leading-tight">
              measured<span className="text-accent">.</span>
            </h2>
            <p className="text-[15px] mt-4 text-foreground/70 leading-relaxed max-w-xs">
              <code className="font-mono text-foreground text-[0.85em]">logmind context</code> is the one-read
              cold-start: spec → file-structure → repomap → timeline, in
              cache-prefix order. It prints a receipt for its own density.
            </p>
          </div>
          <div className="sm:col-span-8">
            <p className="text-[15px] leading-[1.65] text-foreground/85 mb-6">
              Four documents, one read, ordered most-stable-first so an
              agent&apos;s prompt cache actually hits: the{" "}
              <strong>spec</strong> (hand-authored, rarely changes), the{" "}
              <strong>file-structure</strong> (the tree), the{" "}
              <strong>repomap</strong> (signature skeletons, bodies dropped),
              and the <strong>timeline</strong> (newest-first decisions).
              Byte-stable across re-reads — the same task re-reading it pays
              cache rates, not input rates.
            </p>
            <pre className="border border-rule bg-code-bg px-4 py-4 text-[12px] sm:text-sm leading-[1.7] font-mono text-foreground/90 overflow-x-auto">
{`$ logmind context --stats
logmind context — token receipt (est. ~4 chars/token, deterministic)

  payload total:   20486 tok  (spec 167 + file-structure 593 + repomap 7980 + timeline 11564 + framing)
  the repomap distills 179096 tok of Go source -> 22.4x denser.
  the timeline distills 82862 tok of raw decision logs -> 7.2x denser.
  reading this pre-baked payload replaces a git log / ls -R / grep cold-start.
  cache it as a stable prefix -> every re-read costs ~0.1x (about 90% off).`}
            </pre>
            <p className="marginalia normal-case tracking-normal text-foreground/55 mt-4 text-xs leading-relaxed">
              Measured on this repo, this release, with{" "}
              <code className="font-mono">context.repomap: true</code> enabled
              (off by default — leaving it off keeps the payload
              byte-identical for repos that don&apos;t need the Go/TS/JS
              signature skeleton). <code className="font-mono">logmind context --stats</code> ships in the
              binary — no setup, no API keys, no synthetic benchmark.
            </p>
          </div>
        </div>
      </section>

      {/* install */}
      <section
        id="install"
        className="px-6 sm:px-10 lg:px-16 py-20 border-t border-rule"
      >
        <div className="max-w-6xl mx-auto w-full grid grid-cols-1 sm:grid-cols-12 gap-8">
          <div className="sm:col-span-4">
            <h2 className="display text-4xl sm:text-5xl font-light leading-tight">
              install<span className="text-accent">.</span>
            </h2>
            <p className="text-[15px] mt-4 text-foreground/70 leading-relaxed max-w-xs">
              v{CURRENT_VERSION} ships as a single signed + notarized Go binary. Brew or curl,
              pick whichever lives closest to your other dev tools.
            </p>
          </div>
          <div className="sm:col-span-8">
            <CommandBlock cmd={BREW} hint="homebrew" index="01" />
            <CommandBlock cmd={CURL} hint="curl" index="02" />
            <CommandBlock cmd={SKILL} hint="agent skill" index="03" />
            <CommandBlock cmd={VERIFY} hint="verify" index="04" />
            <div className="border-t border-rule" />
            <p className="marginalia normal-case tracking-normal text-foreground/55 mt-6 text-xs leading-relaxed">
              <code className="font-mono">logmind --version</code> prints{" "}
              <code className="font-mono whitespace-pre-line">{VERIFY_OUTPUT}</code> for the current release
              {!IS_NEXT_RELEASED && ` (v${NEXT_VERSION} is in design, not yet released)`}.
              The agent skill (03) is optional but recommended — it teaches
              Claude Code, Cursor, Codex et al. when and how to call{" "}
              <code className="font-mono">logmind log</code> in any project
              that has logmind installed.
            </p>

            {/* Deprecated Python path — kept for v0.6.x consumers migrating off pip */}
            <div className="mt-12 border border-rule/60 bg-code-bg/40 px-5 py-5">
              <div className="marginalia normal-case tracking-normal text-foreground/55 text-xs mb-3 flex flex-wrap items-baseline gap-x-3 gap-y-1">
                <span className="uppercase tracking-[0.18em] text-accent">deprecated</span>
                <span>Python wheel · frozen at v0.6.16</span>
              </div>
              <div className="font-mono text-sm sm:text-[15px] flex items-center justify-between gap-4 group">
                <code className="text-foreground/70 break-all">
                  <span className="text-foreground/40 select-none">$ </span>
                  {PIP_LEGACY}
                </code>
                <CopyButton text={PIP_LEGACY} label="Copy legacy pip command" />
              </div>
              <p className="marginalia normal-case tracking-normal text-foreground/55 mt-4 text-xs leading-relaxed">
                <code className="font-mono">pip install logmind</code> is{" "}
                <strong>frozen at v0.6.16</strong> — the last published
                Python release. New installs should use the Go binary
                above. The PyPI package is kept on PyPI only to honour
                old pinning; it receives no further updates, no security
                backports, and no feature parity with v2.0+. Migrating?
                See the{" "}
                <a
                  href="https://github.com/thrillmade/logmind/blob/main/docs/install.md#deprecated-python-install"
                  className="text-accent hover:underline"
                >
                  migration guide
                </a>{" "}
                for the one-line CI swap.
              </p>
            </div>
          </div>
        </div>
      </section>

      {/* quickstart */}
      <section className="px-6 sm:px-10 lg:px-16 py-20 border-t border-rule">
        <div className="max-w-6xl mx-auto w-full">
          <div className="grid grid-cols-1 sm:grid-cols-12 gap-8 mb-8">
            <div className="sm:col-span-4">
              <h2 className="display text-4xl sm:text-5xl font-light leading-tight">
                <span className="italic font-normal">quick</span>start
                <span className="text-accent">.</span>
              </h2>
              <p className="text-[15px] mt-4 text-foreground/70 leading-relaxed max-w-xs">
                Init once per repo. Then log every meaningful choice. Branch
                routing happens automatically; <code className="font-mono text-foreground text-[0.85em]">-H</code>{" "}
                sets the one-line headline the timeline shows for this
                branch.
              </p>
            </div>
            <div className="sm:col-span-8">
              <div className="border border-rule bg-code-bg">
                <div className="flex items-center justify-between px-4 py-2 border-b border-rule">
                  <span className="marginalia">~ ⁄ feat__auth</span>
                  <CopyButton text={QUICKSTART} label="Copy quickstart" />
                </div>
                <div className="relative">
                  <pre className="px-4 py-4 text-[11px] sm:text-sm overflow-x-auto whitespace-pre leading-[1.7] font-mono text-foreground/90">
                    <code>{QUICKSTART}</code>
                  </pre>
                  {/* Right-edge fade as a visual cue that the block scrolls
                      horizontally on narrow viewports. pointer-events-none
                      so it doesn't block the swipe. */}
                  <div
                    aria-hidden
                    className="pointer-events-none absolute inset-y-0 right-0 w-8 bg-gradient-to-l from-code-bg to-transparent sm:hidden"
                  />
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* coda */}
      <section className="px-6 sm:px-10 lg:px-16 py-24 border-t border-rule">
        <div className="max-w-6xl mx-auto w-full">
          <p className="display italic text-2xl sm:text-3xl text-foreground/85 leading-snug max-w-3xl">
            “Every agent you’ve ever onboarded asked the same questions
            because the answers lived in someone’s head. logmind moves them
            into git, into AGENTS.md, into the{" "}
            <span className="text-accent not-italic">tests that run on every PR</span>
            — so the next agent never has to ask.”
          </p>
          <div className="marginalia mt-6">— design intent</div>
        </div>
      </section>

      {/* footer */}
      <footer className="mt-auto px-6 sm:px-10 lg:px-16 py-16 border-t border-rule">
        <div className="max-w-6xl mx-auto w-full flex flex-col sm:flex-row sm:items-end justify-between gap-8">
          <div>
            <a href="/" className="display text-2xl tracking-tight">
              logmind<span className="text-accent">.</span>
            </a>
            <div className="marginalia normal-case tracking-normal mt-2 text-xs text-foreground/55 flex flex-wrap items-center gap-x-2 gap-y-1">
              {/* Single source of truth: CURRENT_VERSION near the top of
                  this file. The site can't import Go, so this is a
                  hand-maintained mirror of internal/version/version.go —
                  update CURRENT_VERSION (and CURRENT_SPEC /
                  CURRENT_RELEASE_DATE) at every release; nothing else on
                  the page should hardcode a version string. */}
              <span>v{CURRENT_VERSION}</span>
              <span>·</span>
              <span>MIT licensed</span>
              <span>·</span>
              <span>by</span>
              <a
                href="https://thrillmot.com"
                target="_blank"
                rel="noopener noreferrer"
                aria-label="thrllmt (thrillmot.com)"
                className="opacity-60 hover:opacity-100 transition-opacity inline-flex items-center"
              >
                {/* Sized relative to the surrounding text's em so the logo
                    scales proportionally with the marginalia at every
                    viewport. 0.85em matches the cap-height of the adjacent
                    uppercase glyphs. w-auto preserves SVG aspect ratio. */}
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img
                  src="/thrllmt.svg"
                  alt="thrllmt"
                  className="block h-[0.85em] w-auto"
                />
              </a>
            </div>
          </div>
          <div className="flex flex-col sm:items-end gap-4">
            <a
              href="https://www.skills.sh/thrillmade/agent-skills"
              target="_blank"
              rel="noopener noreferrer"
              aria-label="thrillmade/agent-skills collection on skills.sh"
            >
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img
                src="https://skills.sh/b/thrillmade/agent-skills"
                alt="skills.sh skill count"
                height={20}
              />
            </a>
            {/*
              Nav row. At sm:+ (640px), force single-line via whitespace-nowrap
              + inline flow — no wrap, all four dot-separated links on one
              row. At <640px, switch to flex-wrap so links break at element
              boundaries instead of overflowing the viewport.
            */}
            <nav className="text-sm flex flex-wrap items-center gap-y-1 sm:block sm:whitespace-nowrap sm:text-right">
              <NavLink href="https://github.com/thrillmade/logmind/releases">
                changelog
              </NavLink>
              <span aria-hidden className="mx-2 text-foreground/30">·</span>
              <NavLink href="https://github.com/thrillmade/logmind/blob/main/CONTRIBUTING.md">
                contributing
              </NavLink>
              <span aria-hidden className="mx-2 text-foreground/30">·</span>
              <NavLink href="https://github.com/thrillmade/logmind/issues">
                issues
              </NavLink>
              <span aria-hidden className="mx-2 text-foreground/30">·</span>
              <NavLink href="https://github.com/thrillmade/logmind/security/policy">
                security
              </NavLink>
            </nav>
          </div>
        </div>
      </footer>
    </div>
  );
}
