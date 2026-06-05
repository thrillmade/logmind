package skill

import (
	"errors"
	"strings"
	"testing"
)

// visibility_test.go — §8.2 wave-2 layer 4 (repo-visibility) coverage.
//
// CheckRepoVisibility is the testable core; the wire-in to pushWith
// lives in push.go and is exercised by push_test.go. Here we pin:
//
//   - Block path: private source + public target + no flag → Blocked.
//   - Same shape with flag → no block (opt-out works).
//   - Private source + private target → no block (visibility match).
//   - Public source + public target → no block (visibility match).
//   - Unknown source visibility (gh failure) → fail-open, no block.
//   - GHEC "internal" treated as non-public (block alongside "private").
//   - Empty SourceRepo (no remote) → skip source lookup entirely.

func TestCheckRepoVisibility_PrivateSourcePublicTarget_Blocks(t *testing.T) {
	gh := newFakeRunner()
	gh.When([]string{"api", "/repos/acme/private-app", "--jq", ".visibility"},
		runReply{Stdout: "private\n"})
	gh.When([]string{"api", "/repos/thrillmade/agent-skills", "--jq", ".visibility"},
		runReply{Stdout: "public\n"})

	res := CheckRepoVisibility(VisibilityCheckOptions{
		SourceRepo:    "acme/private-app",
		CatalogTarget: "thrillmade/agent-skills",
	}, gh)

	if !res.Blocked {
		t.Fatalf("expected Blocked=true; got %+v", res)
	}
	if res.SourceVisibility != "private" {
		t.Errorf("SourceVisibility = %q; want private", res.SourceVisibility)
	}
	if res.TargetVisibility != "public" {
		t.Errorf("TargetVisibility = %q; want public", res.TargetVisibility)
	}
	if !strings.Contains(res.Reason, "private") || !strings.Contains(res.Reason, "public") {
		t.Errorf("Reason should mention private + public; got %q", res.Reason)
	}
	if !strings.Contains(res.Reason, "allow_promote_from_private") {
		t.Errorf("Reason should name the opt-out flag; got %q", res.Reason)
	}
}

func TestCheckRepoVisibility_AllowPromoteFromPrivate_Unblocks(t *testing.T) {
	gh := newFakeRunner()
	gh.When([]string{"api", "/repos/acme/private-app", "--jq", ".visibility"},
		runReply{Stdout: "private\n"})
	gh.When([]string{"api", "/repos/thrillmade/agent-skills", "--jq", ".visibility"},
		runReply{Stdout: "public\n"})

	res := CheckRepoVisibility(VisibilityCheckOptions{
		SourceRepo:              "acme/private-app",
		CatalogTarget:           "thrillmade/agent-skills",
		AllowPromoteFromPrivate: true,
	}, gh)

	if res.Blocked {
		t.Errorf("opt-out flag should unblock; got %+v", res)
	}
	// Visibility still recorded — caller can still log the cross-vis.
	if res.SourceVisibility != "private" {
		t.Errorf("SourceVisibility should still be recorded; got %q",
			res.SourceVisibility)
	}
	if res.TargetVisibility != "public" {
		t.Errorf("TargetVisibility should still be recorded; got %q",
			res.TargetVisibility)
	}
}

func TestCheckRepoVisibility_PrivateBothEnds_NoBlock(t *testing.T) {
	gh := newFakeRunner()
	gh.When([]string{"api", "/repos/acme/private-app", "--jq", ".visibility"},
		runReply{Stdout: "private"})
	gh.When([]string{"api", "/repos/acme/private-skills", "--jq", ".visibility"},
		runReply{Stdout: "private"})

	res := CheckRepoVisibility(VisibilityCheckOptions{
		SourceRepo:    "acme/private-app",
		CatalogTarget: "acme/private-skills",
	}, gh)

	if res.Blocked {
		t.Errorf("private→private shouldn't block; got %+v", res)
	}
}

func TestCheckRepoVisibility_PublicBothEnds_NoBlock(t *testing.T) {
	gh := newFakeRunner()
	gh.When([]string{"api", "/repos/thrillmade/logmind", "--jq", ".visibility"},
		runReply{Stdout: "public"})
	gh.When([]string{"api", "/repos/thrillmade/agent-skills", "--jq", ".visibility"},
		runReply{Stdout: "public"})

	res := CheckRepoVisibility(VisibilityCheckOptions{
		SourceRepo:    "thrillmade/logmind",
		CatalogTarget: "thrillmade/agent-skills",
	}, gh)

	if res.Blocked {
		t.Errorf("public→public shouldn't block; got %+v", res)
	}
}

func TestCheckRepoVisibility_InternalSource_TreatedAsNonPublic(t *testing.T) {
	// GitHub Enterprise's "internal" visibility — private at the
	// public-web level but visible to the org. We treat it the same
	// way as "private" for cross-vis-push purposes.
	gh := newFakeRunner()
	gh.When([]string{"api", "/repos/ghec-org/internal-app", "--jq", ".visibility"},
		runReply{Stdout: "internal"})
	gh.When([]string{"api", "/repos/thrillmade/agent-skills", "--jq", ".visibility"},
		runReply{Stdout: "public"})

	res := CheckRepoVisibility(VisibilityCheckOptions{
		SourceRepo:    "ghec-org/internal-app",
		CatalogTarget: "thrillmade/agent-skills",
	}, gh)

	if !res.Blocked {
		t.Errorf("internal→public should block (GHEC parity); got %+v", res)
	}
}

func TestCheckRepoVisibility_GhFailure_FailsOpen(t *testing.T) {
	// gh returns an error → visibility empty → no block. The fail-open
	// shape is intentional (see CheckRepoVisibility docstring): layers
	// 1-3 are the load-bearing checks; layer 4 is an extra guard that
	// can't be allowed to fail the push on a transient API blip.
	gh := newFakeRunner()
	gh.When([]string{"api", "/repos/acme/anywhere", "--jq", ".visibility"},
		runReply{Err: errors.New("network down")})
	gh.When([]string{"api", "/repos/thrillmade/agent-skills", "--jq", ".visibility"},
		runReply{Err: errors.New("network down")})

	res := CheckRepoVisibility(VisibilityCheckOptions{
		SourceRepo:    "acme/anywhere",
		CatalogTarget: "thrillmade/agent-skills",
	}, gh)

	if res.Blocked {
		t.Errorf("gh failure should fail-open; got %+v", res)
	}
	if res.SourceVisibility != "" {
		t.Errorf("SourceVisibility on gh error = %q; want empty", res.SourceVisibility)
	}
}

func TestCheckRepoVisibility_EmptySourceSlug_SkipsSourceLookup(t *testing.T) {
	// SourceRepo "" → no remote configured → skip source-visibility
	// fetch. Target lookup still runs (catalog is always known) but
	// without source visibility, the cross-vis decision can't fire
	// and Blocked stays false.
	gh := newFakeRunner()
	gh.When([]string{"api", "/repos/thrillmade/agent-skills", "--jq", ".visibility"},
		runReply{Stdout: "public"})

	res := CheckRepoVisibility(VisibilityCheckOptions{
		SourceRepo:    "",
		CatalogTarget: "thrillmade/agent-skills",
	}, gh)

	if res.Blocked {
		t.Errorf("empty source slug shouldn't block; got %+v", res)
	}
	if res.SourceVisibility != "" {
		t.Errorf("SourceVisibility = %q; want empty (skipped)", res.SourceVisibility)
	}
	if res.TargetVisibility != "public" {
		t.Errorf("TargetVisibility = %q; want public", res.TargetVisibility)
	}
	// Assert no source-side API call fired.
	for _, c := range gh.calls {
		if len(c.Args) >= 2 && c.Args[0] == "api" && strings.Contains(c.Args[1], "/repos/") {
			if !strings.Contains(c.Args[1], "thrillmade/agent-skills") {
				t.Errorf("unexpected source-side API call: %+v", c.Args)
			}
		}
	}
}

func TestCheckRepoVisibility_EmptyTargetSlug_SkipsTargetLookup(t *testing.T) {
	// CatalogTarget "" → target lookup skipped. Edge case: callers
	// normally validate catalog non-empty before this runs, but the
	// function should still be safe under the degenerate input.
	gh := newFakeRunner()
	res := CheckRepoVisibility(VisibilityCheckOptions{
		SourceRepo:    "acme/private-app",
		CatalogTarget: "",
	}, gh)
	if res.Blocked {
		t.Errorf("empty target shouldn't block; got %+v", res)
	}
}

func TestCheckRepoVisibility_BlockReason_NamesBothRepos(t *testing.T) {
	gh := newFakeRunner()
	gh.When([]string{"api", "/repos/acme/private-app", "--jq", ".visibility"},
		runReply{Stdout: "private"})
	gh.When([]string{"api", "/repos/thrillmade/agent-skills", "--jq", ".visibility"},
		runReply{Stdout: "public"})

	res := CheckRepoVisibility(VisibilityCheckOptions{
		SourceRepo:    "acme/private-app",
		CatalogTarget: "thrillmade/agent-skills",
	}, gh)

	// Reason should name both repos so the user understands what
	// pair triggered the block.
	if !strings.Contains(res.Reason, "acme/private-app") {
		t.Errorf("Reason missing source slug; got %q", res.Reason)
	}
	if !strings.Contains(res.Reason, "thrillmade/agent-skills") {
		t.Errorf("Reason missing target slug; got %q", res.Reason)
	}
	if !strings.Contains(res.Reason, "layers 1-3 still run") {
		t.Errorf("Reason should clarify that other layers still run; got %q", res.Reason)
	}
}

// TestIsNonPublic — the helper that drives the block decision.
func TestIsNonPublic(t *testing.T) {
	cases := map[string]bool{
		"public":   false,
		"PUBLIC":   false,
		"private":  true,
		"Private":  true,
		"internal": true,
		"":         false,
		"unknown":  false,
		"  ":       false,
	}
	for in, want := range cases {
		if got := isNonPublic(in); got != want {
			t.Errorf("isNonPublic(%q) = %v; want %v", in, got, want)
		}
	}
}

// TestFetchVisibility_TrimsAndLowercases — the gh stdout shape isn't
// guaranteed to be normalized, so we strip whitespace + casing on the
// way out.
func TestFetchVisibility_TrimsAndLowercases(t *testing.T) {
	gh := newFakeRunner()
	gh.When([]string{"api", "/repos/foo/bar", "--jq", ".visibility"},
		runReply{Stdout: "  PUBLIC \n"})
	got := fetchVisibility(gh, "foo/bar")
	if got != "public" {
		t.Errorf("fetchVisibility normalised = %q; want public", got)
	}
}
