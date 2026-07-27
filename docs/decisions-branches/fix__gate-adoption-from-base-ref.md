← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-26-security-read-the-derived-docs-gate-adoption-signal-from-the -->
- **2026-07-26** — Security: read the derived-docs gate adoption signal from the base ref, never from the PR under judgement
<!-- logmind-entry-end -->

## 2026-07-26 22:03 - Security: read the derived-docs gate adoption signal from the base ref, never from the PR under judgement

**Reasoning:** actions/checkout on a pull_request event lands refs/pull/N/merge — the PR own content — so grepping .logmind/config.yml from the workspace let a pull request DISABLE THE GATE FOR ITSELF by setting mode driver in the same PR that edited a derived doc. That is the self-referential-authority failure the SPEC base-ref MUSTs in 1.12.1 and 10.3.3 exist to prevent, and it was live on main

**Alternatives considered:** Keep the checkout but compare against origin/main in the workspace (rejected: the workspace is still PR-controlled; the fix has to be a ref the PR cannot influence)

**Implications:**
- The PR job now performs NO checkout at all and reads the config at pull_request.base.sha via the API, so only a commit already on the base branch can turn the gate on or off; the template test pins both the base-ref read and the ABSENCE of a checkout, proven non-vacuous by mutation

---

