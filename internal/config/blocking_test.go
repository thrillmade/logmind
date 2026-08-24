// blocking_test.go — the SPEC §1.6 blocking-setting table: which keys it
// covers, and which direction of write counts as a weakening.
package config

import "testing"

func TestBlockingSettings_CoversTheThreeKeysSpecNames(t *testing.T) {
	want := map[string]bool{
		"git.enforce_commits": false,
		"review.strict_mode":  false,
		"review.auto_fix":     false,
	}
	for _, b := range BlockingSettings() {
		if _, ok := want[b.Key]; !ok {
			t.Errorf("BlockingSettings has unexpected key %q — §1.6 names exactly three", b.Key)
			continue
		}
		want[b.Key] = true
	}
	for key, seen := range want {
		if !seen {
			t.Errorf("BlockingSettings is missing %q — §1.6 names it", key)
		}
	}
}

func TestLookupBlocking_OnlyTheNamedKeys(t *testing.T) {
	for _, key := range []string{"git.enforce_commits", "review.strict_mode", "review.auto_fix"} {
		if _, ok := LookupBlocking(key); !ok {
			t.Errorf("LookupBlocking(%q) = not found; want found", key)
		}
	}
	// Controls: keys an agent legitimately writes (§1.6's next paragraph).
	for _, key := range []string{
		"git.commit_line_threshold", "git.auto_push", "agents.claude",
		"enforce_commits", "review", "git.enforce_commits.x",
	} {
		if _, ok := LookupBlocking(key); ok {
			t.Errorf("LookupBlocking(%q) = found; want not found (over-blocking)", key)
		}
	}
}

func TestWeakens_DirectionPerSetting(t *testing.T) {
	cases := []struct {
		key  string
		val  any
		want bool
	}{
		// enforce_commits / strict_mode: the gate is ON when true, so
		// false — or anything that is not true — weakens it.
		{"git.enforce_commits", false, true},
		{"git.enforce_commits", true, false},
		{"git.enforce_commits", 0, true},
		{"git.enforce_commits", "no", true},
		{"review.strict_mode", false, true},
		{"review.strict_mode", true, false},
		// auto_fix: a person pushes the fix when it is OFF, so turning it
		// on — or raising the round cap — is the weakening direction.
		{"review.auto_fix", true, true},
		{"review.auto_fix", 3, true},
		{"review.auto_fix", false, false},
	}
	for _, c := range cases {
		b, ok := LookupBlocking(c.key)
		if !ok {
			t.Fatalf("LookupBlocking(%q) not found", c.key)
		}
		if got := b.Weakens(c.val); got != c.want {
			t.Errorf("%s.Weakens(%#v) = %v; want %v", c.key, c.val, got, c.want)
		}
	}
}

// A slice is not a comparable type; Weakens must classify it, not panic.
func TestWeakens_UncomparableValueDoesNotPanic(t *testing.T) {
	b, _ := LookupBlocking("git.enforce_commits")
	if !b.Weakens([]any{"a", "b"}) {
		t.Errorf("Weakens([]any{...}) = false; want true (not the person-in-loop value)")
	}
}
