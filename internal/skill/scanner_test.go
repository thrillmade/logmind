package skill

import (
	"strings"
	"testing"
)

// scanner_test.go — coverage for §8.2 wave-2 layer 3 (content scanner).
//
// The scanner is a pure function on (body, config) → []hit, so tests
// can exercise every category without spinning up a full pushWith
// call. The integration with the gate decision lives in push_test.go.

// TestScanContent_HardcodedCredentialBaseline pins the canonical
// credential prefixes. Each provider gets one positive case (valid
// token shape → block hit) and one negative case (prose that
// mentions the prefix → no hit). The baseline-block contract means
// these should fire regardless of ScannerConfig.
func TestScanContent_HardcodedCredentialBaseline(t *testing.T) {
	cases := []struct {
		Name    string
		Body    string
		WantHit bool
		// WantKind documents the expected category for positive
		// cases. Empty when WantHit is false.
		WantKind string
	}{
		// Stripe live secret key — 24 alnum after `sk_live_`. Synthetic
		// fixture: embeds DUMMYFIXTURE so commercial secret scanners
		// (GitHub push-protection, etc.) don't flag this test file as
		// containing real credentials. Our regex (8+ alnum) still matches.
		{"stripe-live", "Use sk_live_DUMMYFIXTUREaBcD1234 for billing", true, KindCredential},
		// Stripe restricted key.
		{"stripe-restricted", "rk_live_DUMMYFIXTURE1234567890", true, KindCredential},
		// Stripe pk_live publishable — public-shipping but still
		// fingerprints a real account.
		{"stripe-pk-live", "pk_live_DUMMYFIXTURE1234567890", true, KindCredential},
		// Stripe prose mention — no actual key.
		{"stripe-prose", "Use Stripe sk_live_ keys for billing", false, ""},
		// Slack bot token — `xoxb-` + at least 8 alnum/hyphens.
		{"slack-bot", "Slack token: xoxb-DUMMYFIXTURE-67890-abcdef", true, KindCredential},
		// Slack user token.
		{"slack-user", "xoxp-DUMMYFIXTURE-1234567890-1234567890-abcdef", true, KindCredential},
		{"slack-prose", "Slack provides xoxp tokens for personal auth", false, ""},
		// GitHub PAT (classic) — `ghp_` + 36+ alnum. Synthetic fixture
		// (DUMMYFIXTURE infix) keeps GitHub's secret push-protection
		// from flagging this file while still matching our 36+ regex.
		{"github-pat", "Token: ghp_DUMMYFIXTUREabcdefghijklmnopqrstuv1234", true, KindCredential},
		// GitHub OAuth user token (gho_).
		{"github-oauth", "gho_DUMMYFIXTUREabcdefghijklmnopqrstuv1234", true, KindCredential},
		// GitHub server-to-server (ghs_).
		{"github-s2s", "ghs_DUMMYFIXTUREabcdefghijklmnopqrstuv1234", true, KindCredential},
		// GitHub fine-grained PAT — `github_pat_` + 60+ chars including underscores.
		{"github-finegrained",
			"github_pat_DUMMYFIXTURE_abcdefghijklmnopqrstuvwxyz1234567890abcdefghijklmnop",
			true, KindCredential},
		// GitHub prose mention.
		{"github-prose", "Setting a ghp_ prefixed personal access token", false, ""},
		// npm publish token.
		{"npm-token", "npm_DUMMYFIXTUREabcdefghijklmnopqrstuv1234", true, KindCredential},
		// AWS Access Key ID — `AKIA` + 16 uppercase alnum. Use a
		// publicly documented test-fixture key shape (the AWS
		// SDK uses AKIAIOSFODNN7EXAMPLE as a docs placeholder, but to
		// avoid even that pattern triggering push protection we
		// substitute DUMMY in the middle).
		{"aws-akia", "AWS key: AKIADUMMYFIXTUREXYZ7", true, KindCredential},
		// AWS prose — partial mention.
		{"aws-prose", "AKIA keys are AWS Access Key IDs", false, ""},
		// GCP service account JSON shape.
		{"gcp-sa", `Config: {"type": "service_account", "project_id": "foo"}`, true, KindCredential},
		{"gcp-sa-tight", `{"type":"service_account","project_id":"foo"}`, true, KindCredential},
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			hits := ScanContent([]byte(c.Body), ScannerConfig{})
			gotCredHit := false
			for _, h := range hits {
				if h.Kind == KindCredential {
					gotCredHit = true
					// Baseline credential hits must default to BLOCK.
					if h.Severity != SeverityBlock {
						t.Errorf("credential hit severity = %q; want block",
							h.Severity)
					}
					if h.Layer != scannerLayerName {
						t.Errorf("hit Layer = %q; want %q", h.Layer, scannerLayerName)
					}
					if h.LineNumber < 1 {
						t.Errorf("hit LineNumber = %d; want ≥1", h.LineNumber)
					}
				}
			}
			if gotCredHit != c.WantHit {
				t.Errorf("got credential hit = %v; want %v (body=%q, hits=%+v)",
					gotCredHit, c.WantHit, c.Body, hits)
			}
			_ = c.WantKind
		})
	}
}

// TestScanContent_RedactsCredentialMatch ensures we don't echo live
// token bytes back into the error output. The Match field should
// contain the provider name + a truncated prefix, never the secret tail.
func TestScanContent_RedactsCredentialMatch(t *testing.T) {
	body := "Token: ghp_DUMMYFIXTUREabcdefghijklmnopqrstuv1234TAIL"
	hits := ScanContent([]byte(body), ScannerConfig{})
	if len(hits) == 0 {
		t.Fatalf("expected at least one hit; got none")
	}
	var credHit *PrivacyScannerHit
	for i := range hits {
		if hits[i].Kind == KindCredential {
			credHit = &hits[i]
			break
		}
	}
	if credHit == nil {
		t.Fatalf("no credential hit; got %+v", hits)
	}
	// Provider name appears.
	if !strings.HasPrefix(credHit.Match, "github:") {
		t.Errorf("Match should start with provider name; got %q", credHit.Match)
	}
	// Full token does NOT appear. Tail-marker "TAIL" should be elided.
	if strings.Contains(credHit.Match, "TAIL") {
		t.Errorf("Match leaked the full token tail: %q", credHit.Match)
	}
	// Ellipsis truncation marker present.
	if !strings.Contains(credHit.Match, "…") {
		t.Errorf("Match should be truncated with an ellipsis; got %q", credHit.Match)
	}
}

// TestScanContent_HardcodedKeywordBaseline pins the baseline keyword
// list. Each keyword fires regardless of ScannerConfig.Keywords;
// matching is case-insensitive.
func TestScanContent_HardcodedKeywordBaseline(t *testing.T) {
	for _, kw := range []string{
		"confidential",
		"Confidential",
		"PROPRIETARY",
		"Internal Use Only",
		"do NOT share",
		"NDA",
		"under embargo",
	} {
		t.Run(kw, func(t *testing.T) {
			body := "Some text. " + kw + ". More text."
			hits := ScanContent([]byte(body), ScannerConfig{})
			found := false
			for _, h := range hits {
				if h.Kind == KindKeyword {
					found = true
					if h.Severity != SeverityBlock {
						t.Errorf("keyword hit severity = %q; want block",
							h.Severity)
					}
				}
			}
			if !found {
				t.Errorf("expected keyword hit for %q; got %+v", kw, hits)
			}
		})
	}
}

// TestScanContent_KeywordsExtraAdded checks that user-supplied
// keywords land in the scanner alongside the baseline.
func TestScanContent_KeywordsExtraAdded(t *testing.T) {
	body := "This is project-thunder content."
	// Without config, no hit.
	if hits := ScanContent([]byte(body), ScannerConfig{}); hitsKindCount(hits, KindKeyword) != 0 {
		t.Errorf("baseline shouldn't catch project-thunder; got %+v", hits)
	}
	// With config addition, hit fires.
	hits := ScanContent([]byte(body), ScannerConfig{
		Keywords: []string{"project-thunder"},
	})
	if hitsKindCount(hits, KindKeyword) == 0 {
		t.Errorf("user keyword 'project-thunder' should have fired; got %+v", hits)
	}
}

// TestScanContent_KeywordsExtraCantWeakenBaseline ensures the
// user-supplied keyword list can ADD to the baseline but can't REMOVE
// the baseline entries. We test the additive guarantee by verifying
// baseline still fires even when config is set.
func TestScanContent_KeywordsExtraCantWeakenBaseline(t *testing.T) {
	body := "This is confidential content."
	// Even with a totally unrelated keyword config, "confidential"
	// still fires because it's hardcoded.
	hits := ScanContent([]byte(body), ScannerConfig{
		Keywords: []string{"something-else"},
	})
	if hitsKindCount(hits, KindKeyword) == 0 {
		t.Errorf("baseline keyword 'confidential' should still fire under config; got %+v",
			hits)
	}
}

// TestScanContent_OrgDomainsOptInOnly verifies the no-baseline
// behaviour: without ScannerConfig.OrgDomains, no org-domain hit
// fires regardless of what the body contains.
func TestScanContent_OrgDomainsOptInOnly(t *testing.T) {
	body := "Visit api.thrillmade.internal for the secrets dashboard."
	if hits := ScanContent([]byte(body), ScannerConfig{}); hitsKindCount(hits, KindOrgDomain) != 0 {
		t.Errorf("org-domain category should have no baseline; got %+v", hits)
	}
}

// TestScanContent_OrgDomainsWithConfig fires when configured.
func TestScanContent_OrgDomainsWithConfig(t *testing.T) {
	body := "Visit api.thrillmade.internal for the secrets dashboard."
	hits := ScanContent([]byte(body), ScannerConfig{
		OrgDomains: []string{"thrillmade.internal"},
	})
	if hitsKindCount(hits, KindOrgDomain) == 0 {
		t.Errorf("org-domain configured but didn't fire; got %+v", hits)
	}
	// Severity defaults to warn (org-domain).
	for _, h := range hits {
		if h.Kind != KindOrgDomain {
			continue
		}
		if h.Severity != SeverityWarn {
			t.Errorf("org-domain default severity = %q; want warn", h.Severity)
		}
	}
}

// TestScanContent_OrgDomainsBareMention skips a bare domain mention
// (no subdomain prefix) — only host-style references fire.
func TestScanContent_OrgDomainsBareMention(t *testing.T) {
	body := "Our internal network uses thrillmade.internal as a TLD."
	hits := ScanContent([]byte(body), ScannerConfig{
		OrgDomains: []string{"thrillmade.internal"},
	})
	if hitsKindCount(hits, KindOrgDomain) != 0 {
		t.Errorf("bare-domain prose should not fire; got %+v", hits)
	}
}

// TestScanContent_OrgDomainsMalformedRegexSkipped exercises the
// fail-safe path for a user-supplied domain that doesn't compile. We
// can't easily construct an actually-malformed input because
// QuoteMeta escapes everything, but we can prove an empty/whitespace
// entry is skipped without error.
func TestScanContent_OrgDomainsMalformedRegexSkipped(t *testing.T) {
	body := "api.thrillmade.internal"
	hits := ScanContent([]byte(body), ScannerConfig{
		OrgDomains: []string{"", "   ", "thrillmade.internal"},
	})
	// Whitespace/empty entries get skipped; only the real entry fires.
	gotOrgDomain := hitsKindCount(hits, KindOrgDomain)
	if gotOrgDomain != 1 {
		t.Errorf("expected 1 org-domain hit (empty/whitespace skipped); got %d in %+v",
			gotOrgDomain, hits)
	}
}

// TestScanContent_LocalPathsDefaultWarn verifies that
// `/Users/<name>/` and `/home/<name>/` style references fire at
// "warn" severity. The push gate prints these but doesn't reject.
func TestScanContent_LocalPathsDefaultWarn(t *testing.T) {
	cases := []string{
		"See /Users/alice/projects/foo/bar for the script",
		"Clone to /home/bob/repos/myskill first",
	}
	for _, body := range cases {
		t.Run(body, func(t *testing.T) {
			hits := ScanContent([]byte(body), ScannerConfig{})
			if hitsKindCount(hits, KindLocalPath) == 0 {
				t.Errorf("local-path should fire; got %+v", hits)
			}
			for _, h := range hits {
				if h.Kind == KindLocalPath && h.Severity != SeverityWarn {
					t.Errorf("local-path default severity = %q; want warn",
						h.Severity)
				}
			}
		})
	}
}

// TestScanContent_LocalPathsCanonicalIgnored — `/tmp/`, `/var/`,
// `/etc/`, and canonical project paths don't fire. We don't want to
// false-positive on legitimate system paths.
func TestScanContent_LocalPathsCanonicalIgnored(t *testing.T) {
	for _, body := range []string{
		"Logs land in /tmp/log",
		"Edit /etc/config",
		"Output to /var/run/foo",
		"In ./docs/timeline.md, see the note",
	} {
		t.Run(body, func(t *testing.T) {
			hits := ScanContent([]byte(body), ScannerConfig{})
			if hitsKindCount(hits, KindLocalPath) != 0 {
				t.Errorf("non-home path triggered local-path: %+v", hits)
			}
		})
	}
}

// TestScanContent_LineNumbersReported pins that line numbers are
// 1-based and correct under multi-line input.
func TestScanContent_LineNumbersReported(t *testing.T) {
	body := "line one\n" +
		"line two\n" +
		"sk_live_DUMMYFIXTUREaBcD1234\n" +
		"line four\n"
	hits := ScanContent([]byte(body), ScannerConfig{})
	if len(hits) == 0 {
		t.Fatalf("expected a hit on line 3; got none")
	}
	if hits[0].LineNumber != 3 {
		t.Errorf("LineNumber = %d; want 3 (body=%q)",
			hits[0].LineNumber, body)
	}
}

// TestScanContent_SeverityOverrideWidens verifies that the user can
// promote a "warn" category to "block" via SeverityOverrides.
func TestScanContent_SeverityOverrideWidens(t *testing.T) {
	body := "/Users/alice/foo"
	hits := ScanContent([]byte(body), ScannerConfig{
		SeverityOverrides: map[string]string{KindLocalPath: SeverityBlock},
	})
	if len(hits) == 0 {
		t.Fatalf("expected a local-path hit; got none")
	}
	for _, h := range hits {
		if h.Kind != KindLocalPath {
			continue
		}
		if h.Severity != SeverityBlock {
			t.Errorf("local-path override to block failed; got %q",
				h.Severity)
		}
	}
}

// TestScanContent_SeverityOverrideCantWeakenBaseline is the contract
// guarantee from the master plan: credential + keyword baseline are
// always BLOCK. Even if config tries to set them to WARN, the actual
// severity stays at block.
func TestScanContent_SeverityOverrideCantWeakenBaseline(t *testing.T) {
	cases := []struct {
		Kind string
		Body string
	}{
		{KindCredential, "ghp_DUMMYFIXTUREabcdefghijklmnopqrstuv1234"},
		{KindKeyword, "this is confidential"},
	}
	for _, c := range cases {
		t.Run(c.Kind, func(t *testing.T) {
			hits := ScanContent([]byte(c.Body), ScannerConfig{
				SeverityOverrides: map[string]string{c.Kind: SeverityWarn},
			})
			if len(hits) == 0 {
				t.Fatalf("expected at least one hit; got none")
			}
			for _, h := range hits {
				if h.Kind != c.Kind {
					continue
				}
				if h.Severity != SeverityBlock {
					t.Errorf("baseline-block %q got weakened to %q despite override",
						c.Kind, h.Severity)
				}
			}
		})
	}
}

// TestScanContent_SeverityOverrideIgnoresBadValues ensures a typo'd
// override ("info", empty string, random text) doesn't crash or
// corrupt severities.
func TestScanContent_SeverityOverrideIgnoresBadValues(t *testing.T) {
	body := "/Users/alice/foo"
	hits := ScanContent([]byte(body), ScannerConfig{
		SeverityOverrides: map[string]string{KindLocalPath: "spaghetti"},
	})
	for _, h := range hits {
		if h.Kind != KindLocalPath {
			continue
		}
		// Bad override value falls back to default "warn".
		if h.Severity != SeverityWarn {
			t.Errorf("bad override should fall back to default; got %q",
				h.Severity)
		}
	}
}

// TestScanContent_EmptyBodyReturnsNil — defensive contract.
func TestScanContent_EmptyBodyReturnsNil(t *testing.T) {
	if hits := ScanContent(nil, ScannerConfig{}); hits != nil {
		t.Errorf("nil body returned %+v; want nil", hits)
	}
	if hits := ScanContent([]byte(""), ScannerConfig{}); hits != nil {
		t.Errorf("empty body returned %+v; want nil", hits)
	}
}

// TestScanContent_AllHitsReturned ensures the scanner doesn't
// short-circuit on first match — every category contributing to the
// rejection should land in one error message.
func TestScanContent_AllHitsReturned(t *testing.T) {
	body := strings.Join([]string{
		"Confidential — do not share.",
		"Token: ghp_DUMMYFIXTUREabcdefghijklmnopqrstuv1234",
		"See /Users/alice/foo/bar",
		"Reference: api.thrillmade.internal",
	}, "\n")
	hits := ScanContent([]byte(body), ScannerConfig{
		OrgDomains: []string{"thrillmade.internal"},
	})
	// We expect: keyword "confidential", keyword "do not share",
	// credential ghp_*, local-path /Users/alice/, org-domain
	// api.thrillmade.internal. At minimum 5 hits.
	if len(hits) < 5 {
		t.Errorf("expected ≥5 hits across categories; got %d:\n%+v",
			len(hits), hits)
	}
	// Each category present.
	for _, kind := range []string{KindCredential, KindKeyword, KindLocalPath, KindOrgDomain} {
		if hitsKindCount(hits, kind) == 0 {
			t.Errorf("missing hit for %q in:\n%+v", kind, hits)
		}
	}
}

// TestBlockingHits_AndWarningHits — convenience filters.
func TestBlockingHits_AndWarningHits(t *testing.T) {
	hits := []PrivacyScannerHit{
		{Kind: KindCredential, Severity: SeverityBlock},
		{Kind: KindLocalPath, Severity: SeverityWarn},
		{Kind: KindKeyword, Severity: SeverityBlock},
	}
	if got := len(BlockingHits(hits)); got != 2 {
		t.Errorf("BlockingHits count = %d; want 2", got)
	}
	if got := len(WarningHits(hits)); got != 1 {
		t.Errorf("WarningHits count = %d; want 1", got)
	}
	if BlockingHits(nil) != nil {
		t.Errorf("BlockingHits(nil) should be nil")
	}
	if WarningHits(nil) != nil {
		t.Errorf("WarningHits(nil) should be nil")
	}
}

// TestMergeKeywords_DedupesCaseInsensitive — internal helper.
func TestMergeKeywords_DedupesCaseInsensitive(t *testing.T) {
	got := mergeKeywords(
		[]string{"confidential", "NDA"},
		[]string{"CONFIDENTIAL", "project-x"},
	)
	want := []string{"confidential", "nda", "project-x"}
	if len(got) != len(want) {
		t.Fatalf("merge got %d entries; want %d (got=%v)", len(got), len(want), got)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("merge[%d] = %q; want %q", i, got[i], want[i])
		}
	}
}

// hitsKindCount counts hits matching a kind. Used across tests to
// avoid open-coding the loop.
func hitsKindCount(hits []PrivacyScannerHit, kind string) int {
	count := 0
	for _, h := range hits {
		if h.Kind == kind {
			count++
		}
	}
	return count
}
