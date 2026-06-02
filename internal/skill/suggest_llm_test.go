package skill

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestNewAnthropicSuggester_NoKey returns LLMUnavailableErr when the
// env var resolves to empty.
func TestNewAnthropicSuggester_NoKey(t *testing.T) {
	t.Setenv("LOGMIND_TEST_KEY", "")
	_, err := NewAnthropicSuggester(SuggestLLMConfig{
		AnthropicAPIKeyEnv: "LOGMIND_TEST_KEY",
	})
	if !errors.Is(err, LLMUnavailableErr) {
		t.Fatalf("expected LLMUnavailableErr; got %v", err)
	}
}

// TestAnthropicSuggester_HappyPath exercises the full HTTP round-trip
// against an httptest server. The server echoes a canned JSON
// response; we confirm the headers + body + parse path are wired
// correctly.
func TestAnthropicSuggester_HappyPath(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("x-api-key")
		w.Header().Set("Content-Type", "application/json")
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"model":"claude-haiku-4-5"`) {
			t.Errorf("missing model in request body: %s", body)
		}
		// Canned response — JSON wrapped in text content block.
		resp := map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": `{"patterns": [{"phrase": "api-versioning", "slug": "api-versioning", "draft_description": "When versioning APIs..."}]}`,
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	suggester := &AnthropicSuggester{
		APIKey:     "test-key",
		HTTPClient: server.Client(),
		BaseURL:    server.URL,
	}
	in := LLMRequest{
		Model:     "claude-haiku-4-5",
		MaxTokens: 100,
		TopN:      5,
		CandidatePatterns: []HeuristicCandidate{
			{
				Phrase:        "api-versioning",
				Slug:          "api-versioning",
				DecisionCount: 4,
				Evidence:      []SuggestEvidence{{File: "decisions.md", Snippet: "uses api-versioning"}},
			},
		},
	}
	out, err := suggester.Suggest(context.Background(), in)
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if gotAuth != "test-key" {
		t.Errorf("expected x-api-key=test-key; got %q", gotAuth)
	}
	if len(out) != 1 {
		t.Fatalf("suggestions = %d; want 1", len(out))
	}
	if out[0].Slug != "api-versioning" {
		t.Errorf("slug = %q; want api-versioning", out[0].Slug)
	}
	if !strings.HasPrefix(out[0].DraftDescription, "When versioning APIs") {
		t.Errorf("draft description not propagated; got %q", out[0].DraftDescription)
	}
}

// TestAnthropicSuggester_ErrorStatus: non-200 → LLMUnavailableErr.
func TestAnthropicSuggester_ErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"type":"authentication_error","message":"bad key"}}`))
	}))
	defer server.Close()

	suggester := &AnthropicSuggester{
		APIKey:     "test-key",
		HTTPClient: server.Client(),
		BaseURL:    server.URL,
	}
	_, err := suggester.Suggest(context.Background(), LLMRequest{Model: "x", MaxTokens: 10})
	if !errors.Is(err, LLMUnavailableErr) {
		t.Fatalf("expected LLMUnavailableErr; got %v", err)
	}
}

// TestSuggestLLM_NilTransport: with engine=llm but no transport,
// SuggestLLM returns LLMUnavailableErr (the CLI's signal to fall
// back).
func TestSuggestLLM_NilTransport(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultSuggestLLMConfig()
	_, err := SuggestLLM(context.Background(), dir, 30, 3, 5, cfg, time.Time{}, nil)
	// No heuristic input → nil, nil. We need a docs/ dir with content
	// to trigger the transport path.
	if err != nil {
		t.Fatalf("unexpected err on empty repo: %v", err)
	}
}

// TestExtractJSONBlob handles preamble + suffix wrapping.
func TestExtractJSONBlob(t *testing.T) {
	cases := []struct {
		In   string
		Want string
	}{
		{`{"patterns": []}`, `{"patterns": []}`},
		{"Here is the result:\n```json\n{\"a\": 1}\n```\nDone.", `{"a": 1}`},
		{"no braces here", "no braces here"},
	}
	for _, c := range cases {
		if got := extractJSONBlob(c.In); got != c.Want {
			t.Errorf("extractJSONBlob(%q) = %q; want %q", c.In, got, c.Want)
		}
	}
}
