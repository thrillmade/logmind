import { CopyButton } from "./copy-button";

const PIP = "pip install logmind";
const BREW = "brew tap thrillmade/logmind && brew install logmind";
const SKILL = "npx skills add -g thrillmade/logmind-skill";

const QUICKSTART = `$ logmind init
$ git checkout -b feat/auth
$ logmind log "JWT for stateless API auth" \\
    -r "horizontal scaling without session store" \\
    -a "server sessions in Redis" \\
    -i "rotate signing keys quarterly"
✓ wrote docs/decisions-branches/feat__auth.md`;

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
            <NavLink href="https://pypi.org/project/logmind/">pypi</NavLink>
            <NavLink href="https://skills.sh/thrillmade/logmind-skill">skill</NavLink>
          </nav>
        </div>
      </header>

      {/* hero */}
      <section className="px-6 sm:px-10 lg:px-16 pt-12 sm:pt-24 pb-20">
        <div className="max-w-6xl mx-auto w-full">
          <div className="rise marginalia mb-6">
            v0.5.12 ⁄ released 2026-05-30 ⁄ MIT
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
                    <em>they update together</em>. Rebase against main? Fresh
                    clone? Multi-commit amend? They stay synced. CI never
                    catches you with a stale derived doc.
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
                    why-behind-the-code. Onboarding a new agent takes{" "}
                    <em>one read</em>.
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

      {/* measured */}
      <section className="px-6 sm:px-10 lg:px-16 py-20 border-t border-rule">
        <div className="max-w-6xl mx-auto w-full grid grid-cols-1 sm:grid-cols-12 gap-8">
          <div className="sm:col-span-4">
            <h2 className="display text-4xl sm:text-5xl font-light leading-tight">
              measured<span className="text-accent">.</span>
            </h2>
            <p className="text-[15px] mt-4 text-foreground/70 leading-relaxed max-w-xs">
              Logmind costs fewer tokens than the git workflow it replaces.
              Anyone can run the benchmarks. Every release.
            </p>
          </div>
          <div className="sm:col-span-8">
            <p className="text-[15px] leading-[1.65] text-foreground/85 mb-6">
              Decision logging only matters if it stays cheaper than the
              alternative. We measure four ways every release:
              <em> per call</em>,{" "}
              <em>worst case</em>, <em>per session</em>,{" "}
              <em>org cumulative</em>. CI gates on net-saver across all four.
            </p>
            <pre className="border border-rule bg-code-bg px-4 py-4 text-[12px] sm:text-sm leading-[1.7] font-mono text-foreground/90 overflow-x-auto">
{`$ python -m bench
  per-call       -18% bytes vs git equivalent      ✅ saver
  worst-case     -58% even on never-read           ✅ saver
  per-session     informational (4-angle frame)    ℹ info
  org-cumulative  informational (rollup)            ℹ info
ok: 4-angle Q7-logmind compliance`}
            </pre>
            <p className="marginalia normal-case tracking-normal text-foreground/55 mt-4 text-xs leading-relaxed">
              The two informational angles share a thin baseline (read-event
              accounting needs aggregation); the two gating angles are the
              load-bearing checks. <code className="font-mono">python -m bench</code> ships in the repo —
              no setup, no API keys.
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
              Three channels. The same package. Pick whichever lives closest to
              your other dev tools.
            </p>
          </div>
          <div className="sm:col-span-8">
            <CommandBlock cmd={PIP} hint="pip" index="01" />
            <CommandBlock cmd={BREW} hint="homebrew" index="02" />
            <CommandBlock cmd={SKILL} hint="agent skill" index="03" />
            <div className="border-t border-rule" />
            <p className="marginalia normal-case tracking-normal text-foreground/55 mt-6 text-xs leading-relaxed">
              The agent skill is optional but recommended — it teaches Claude
              Code, Cursor, Codex et al. when and how to call <code className="font-mono">logmind log</code> in
              any project that has logmind installed.
            </p>
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
                routing happens automatically.
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
              {/* keep version in sync with pyproject.toml */}
              <span>v0.5.12</span>
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
              <NavLink href="https://github.com/thrillmade/logmind/blob/main/CHANGELOG.md">
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
