package skill

import (
	"strings"
	"testing"
)

// TestCheckFrontmatter_MissingHeader covers the no-frontmatter branch.
func TestCheckFrontmatter_MissingHeader(t *testing.T) {
	r := CheckFrontmatter("# Hello\n")
	if r.OK {
		t.Fatalf("expected failure; got ok")
	}
	if r.Message != "SKILL.md must start with YAML frontmatter (--- block)" {
		t.Errorf("unexpected message: %q", r.Message)
	}
}

// TestCheckFrontmatter_Unterminated covers the missing-closing-dash branch.
func TestCheckFrontmatter_Unterminated(t *testing.T) {
	r := CheckFrontmatter("---\nname: foo\ndescription: bar\n")
	if r.OK {
		t.Fatalf("expected failure; got ok")
	}
	if r.Message != "SKILL.md frontmatter is unterminated (missing closing ---)" {
		t.Errorf("unexpected message: %q", r.Message)
	}
}

// TestCheckFrontmatter_MissingName covers the no-name-field branch.
//
// PR #92 review fix: the anchored regex must NOT match nested keys
// like `domain_name`. We pin the negative case here so a regression
// to substring-match brings the test down.
func TestCheckFrontmatter_MissingName(t *testing.T) {
	body := "---\ndescription: bar\ndomain_name: nope\n---\n# Body\n"
	r := CheckFrontmatter(body)
	if r.OK {
		t.Fatalf("expected failure on missing name; got ok")
	}
	if !strings.Contains(r.Message, "missing required field: name") {
		t.Errorf("unexpected message: %q", r.Message)
	}
}

// TestCheckFrontmatter_MissingDescription covers the no-description branch.
func TestCheckFrontmatter_MissingDescription(t *testing.T) {
	body := "---\nname: ok\npackage_description: nope\n---\n# Body\n"
	r := CheckFrontmatter(body)
	if r.OK {
		t.Fatalf("expected failure on missing description; got ok")
	}
	if !strings.Contains(r.Message, "missing required field: description") {
		t.Errorf("unexpected message: %q", r.Message)
	}
}

// TestCheckFrontmatter_OK covers the happy path with both fields.
func TestCheckFrontmatter_OK(t *testing.T) {
	body := "---\nname: ok\ndescription: yes\n---\n# Body\n"
	r := CheckFrontmatter(body)
	if !r.OK {
		t.Fatalf("expected ok; got %q", r.Message)
	}
}

// TestCheckSizeCap_Under verifies the standard byte-report.
func TestCheckSizeCap_Under(t *testing.T) {
	r := CheckSizeCap("hello", 100)
	if !r.OK {
		t.Fatalf("expected ok; got %q", r.Message)
	}
	if r.Message != "5 bytes (cap: 100)" {
		t.Errorf("unexpected message: %q", r.Message)
	}
}

// TestCheckSizeCap_Over verifies the over-cap byte-report text.
func TestCheckSizeCap_Over(t *testing.T) {
	body := strings.Repeat("x", 200)
	r := CheckSizeCap(body, 100)
	if r.OK {
		t.Fatalf("expected failure; got ok")
	}
	if !strings.HasPrefix(r.Message, "SKILL.md is 200 bytes — over the 100-byte logmind cap.") {
		t.Errorf("unexpected message: %q", r.Message)
	}
}
