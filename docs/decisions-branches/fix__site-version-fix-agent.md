← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-08-01-site-stop-advertising-v2-0-0-as-released-make-version-exampl -->
- **2026-08-01** — site: stop advertising v2.0.0 as released; make --version example match the actual v1.2.0 binary
<!-- logmind-entry-end -->

## 2026-08-01 01:10 - site: stop advertising v2.0.0 as released; make --version example match the actual v1.2.0 binary

**Reasoning:** logmind.dev claimed v2.0.0 was released (hero badge, --version example, footer) and described the commit-guard/PreToolUse enforcement as already active, while the latest tag is v1.2.0 and that enforcement code landed a month after the v1.2.0 tag — a reader following install instructions and running --version got a string that didn't match the page, and the enforced section promised a capability that isn't installable yet

**Alternatives considered:** strip all v2.0.0 content until it ships

**Implications:**
- site now derives every version string from one CURRENT_VERSION/CURRENT_SPEC/CURRENT_RELEASE_DATE block in site/app/page.tsx; flipping those three lines at v2.0.0 tag time clears every forthcoming/in-design caveat automatically (IS_NEXT_RELEASED derives from CURRENT_VERSION === NEXT_VERSION) — see the comment above the constants block

---

