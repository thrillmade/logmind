import { CopyButton } from "./copy-button";

const PIP = "pip install logmind";
const BREW = "brew tap thrillmot/logmind && brew install logmind";
const SKILL = "npx skills add -g thrillmot/logmind-skill";

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
      <header className="px-6 sm:px-10 lg:px-16 py-6 flex items-center justify-between">
        <a href="/" className="display text-xl tracking-tight">
          logmind<span className="text-accent">.</span>
        </a>
        <nav className="flex items-center gap-6 sm:gap-8">
          <NavLink href="https://github.com/thrillmot/logmind">github</NavLink>
          <NavLink href="https://pypi.org/project/logmind/">pypi</NavLink>
          <NavLink href="https://skills.sh/thrillmot/logmind-skill">skill</NavLink>
        </nav>
      </header>

      {/* hero */}
      <section className="px-6 sm:px-10 lg:px-16 pt-12 sm:pt-24 pb-20">
        <div className="max-w-6xl mx-auto w-full">
          <div className="rise marginalia mb-6">
            v0.1 ⁄ released 2026-05-15 ⁄ MIT
          </div>
          <h1 className="rise display text-[12vw] sm:text-[8.5vw] leading-[0.92] font-light max-w-[16ch]" style={{ animationDelay: "0.05s" }}>
            Decisions,
            <br />
            <span className="italic font-normal">captured</span>
            <span className="text-accent">.</span>
            <br />
            <span className="text-muted">while you work.</span>
          </h1>

          <div className="rise mt-12 grid sm:grid-cols-12 gap-8 sm:gap-12" style={{ animationDelay: "0.18s" }}>
            <div className="sm:col-span-1 marginalia hidden sm:block pt-2">
              <span className="block">¶ 01</span>
              <span className="block text-muted/60">brief</span>
            </div>
            <p className="lead sm:col-span-7 text-lg leading-[1.55] text-foreground/85">
              An opinionated decision log for AI-assisted development. One CLI
              command per architectural choice. Branch-aware by default.
              <span className="text-accent"> AGENTS.md </span>
              canonical. Plugs into the agents you already use — Claude Code,
              Cursor, Codex, Cline, Continue — with no per-tool drift.
            </p>
            <aside className="sm:col-span-4 sm:border-l sm:border-rule sm:pl-6 marginalia space-y-3 self-start pt-2">
              <div>
                <span className="block">made for</span>
                <span className="block text-foreground/85 normal-case tracking-normal">
                  developers shipping with AI agents
                </span>
              </div>
              <div>
                <span className="block">opposite of</span>
                <span className="block text-foreground/85 normal-case tracking-normal">
                  ADRs you write at the end of the quarter
                </span>
              </div>
            </aside>
          </div>

          <div className="rise mt-12 flex items-center gap-4 text-sm" style={{ animationDelay: "0.32s" }}>
            <a
              href="https://github.com/thrillmot/logmind"
              target="_blank"
              rel="noopener noreferrer"
              className="font-mono uppercase tracking-[0.18em] px-5 py-3 bg-paper text-background hover:bg-accent hover:text-paper transition-colors"
            >
              read the source →
            </a>
            <a
              href="#install"
              className="font-mono uppercase tracking-[0.18em] px-5 py-3 border border-rule hover:border-accent hover:text-accent transition-colors"
            >
              install ↓
            </a>
          </div>
        </div>
      </section>

      {/* what — three principles */}
      <section className="px-6 sm:px-10 lg:px-16 py-20 border-t border-rule">
        <div className="max-w-6xl mx-auto w-full">
          <div className="ornament marginalia mb-12">
            <span>three principles</span>
          </div>

          <div className="grid sm:grid-cols-3 gap-x-10 gap-y-12">
            {[
              {
                n: "i.",
                title: "branch-aware",
                body: (
                  <>
                    Decisions on a feature branch land in{" "}
                    <code className="font-mono text-accent text-[0.85em]">
                      docs/decisions-branches/&lt;branch&gt;.md
                    </code>
                    . On PR merge, an action appends a one-line summary to{" "}
                    <code className="font-mono text-foreground text-[0.85em]">
                      docs/decisions.md
                    </code>{" "}
                    with the PR link. No more conflicts on the shared file.
                  </>
                ),
              },
              {
                n: "ii.",
                title: "agents.md-first",
                body: (
                  <>
                    A single canonical instruction file. CLAUDE.md, .cursorrules,
                    .windsurfrules, .clinerules, AGENTS.md — nine of them become
                    <em> two-line stubs</em> pointing at the canonical source.
                    No drift, no duplication.
                  </>
                ),
              },
              {
                n: "iii.",
                title: "link integrity",
                body: (
                  <>
                    A GitHub Action ships with{" "}
                    <code className="font-mono text-foreground text-[0.85em]">logmind init</code>
                    : every relative{" "}
                    <code className="font-mono text-foreground text-[0.85em]">
                      [link](path.md)
                    </code>{" "}
                    between docs is verified on every PR. A rename can never
                    silently orphan an agent's context again.
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

      {/* install */}
      <section
        id="install"
        className="px-6 sm:px-10 lg:px-16 py-20 border-t border-rule"
      >
        <div className="max-w-6xl mx-auto w-full grid sm:grid-cols-12 gap-8">
          <div className="sm:col-span-4">
            <div className="marginalia mb-3">section ii</div>
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
          <div className="grid sm:grid-cols-12 gap-8 mb-8">
            <div className="sm:col-span-4">
              <div className="marginalia mb-3">section iii</div>
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
                <pre className="px-4 py-4 text-sm overflow-x-auto whitespace-pre leading-[1.7] font-mono text-foreground/90">
                  <code>{QUICKSTART}</code>
                </pre>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* coda */}
      <section className="px-6 sm:px-10 lg:px-16 py-24 border-t border-rule">
        <div className="max-w-6xl mx-auto w-full">
          <p className="display italic text-2xl sm:text-3xl text-foreground/85 leading-snug max-w-3xl">
            “The why behind the code disappears faster than the what. logmind
            makes that disappearance{" "}
            <span className="text-accent not-italic">expensive</span> on
            purpose.”
          </p>
          <div className="marginalia mt-6">— project README</div>
        </div>
      </section>

      {/* footer */}
      <footer className="mt-auto px-6 sm:px-10 lg:px-16 py-10 border-t border-rule">
        <div className="max-w-6xl mx-auto w-full flex flex-col sm:flex-row items-start sm:items-end justify-between gap-6">
          <div>
            <a href="/" className="display text-2xl tracking-tight">
              logmind<span className="text-accent">.</span>
            </a>
            <div className="marginalia normal-case tracking-normal mt-1 text-xs text-foreground/50">
              v0.1 · MIT licensed · made for things you’d miss if forgotten.
            </div>
          </div>
          <div className="flex flex-wrap gap-x-6 gap-y-3 text-sm">
            <NavLink href="https://github.com/thrillmot/logmind/blob/main/CHANGELOG.md">
              changelog
            </NavLink>
            <NavLink href="https://github.com/thrillmot/logmind/blob/main/CONTRIBUTING.md">
              contributing
            </NavLink>
            <NavLink href="https://github.com/thrillmot/logmind/issues">
              issues
            </NavLink>
            <NavLink href="https://github.com/thrillmot/logmind/security/policy">
              security
            </NavLink>
          </div>
        </div>
      </footer>
    </div>
  );
}
