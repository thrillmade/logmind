package skill

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/thrillmade/logmind/internal/atomicio"
)

// LLMSuggester is the network-isolatable adapter we call from the
// suggest command when engine=llm. Defined as an interface so tests
// can stub the network round-trip without touching the Anthropic API.
//
// Why a single Suggest method vs separate Setup/Cleanup: matches the
// short-lived nature of one CLI invocation. The HTTP client lives in
// the implementation; the interface stays no-state.
type LLMSuggester interface {
	Suggest(ctx context.Context, in LLMRequest) ([]Suggestion, error)
}

// LLMRequest is the bundle of inputs the LLM gets. Carries the
// candidate-token set plus a small per-token evidence sample so the
// model can rank without seeing the entire decision corpus (cost
// guardrail).
type LLMRequest struct {
	CandidatePatterns []HeuristicCandidate
	MaxTokens         int
	Model             string
	TopN              int
}

// HeuristicCandidate is the per-pattern bundle the heuristic stage
// hands to the LLM stage. Provides the LLM enough context to rank
// without dumping the whole decision corpus into the prompt.
type HeuristicCandidate struct {
	Phrase        string
	Slug          string
	DecisionCount int
	Evidence      []SuggestEvidence
}

// LLMUnavailableErr is the sentinel returned when the configured LLM
// engine can't run (missing API key, missing transport, etc.). The
// CLI layer translates this into the graceful-fallback path that
// re-runs the heuristic engine and prints an actionable message.
//
// A typed error (vs a bool flag on the result) keeps the SuggestLLM
// signature clean and lets callers chain via errors.Is.
var LLMUnavailableErr = errors.New("logmind: LLM engine unavailable")

// maxLLMResponseBytes caps the Anthropic HTTP response body. The
// HTTPClient timeout limits elapsed time but not bytes, so without a
// LimitReader a misbehaving (or compromised) endpoint can stream
// indefinitely large bodies and exhaust process memory. 1 MB is
// roughly 100× the expected JSON size at max_tokens=2000.
const maxLLMResponseBytes = 1 << 20 // 1 MB

// SuggestLLMConfig captures the .logmind/config.yml `skill_suggest`
// section. Mirrors the YAML shape spelled out in the v0.6.x plan:
//
//	skill_suggest:
//	  engine: llm                 # llm | heuristic (default: llm)
//	  llm_model: claude-haiku-4-5
//	  llm_max_tokens: 2000
//	  anthropic_api_key_env: ANTHROPIC_API_KEY
//	  fallback_to_heuristic_on_no_key: true
type SuggestLLMConfig struct {
	Engine                     string
	LLMModel                   string
	LLMMaxTokens               int
	AnthropicAPIKeyEnv         string
	FallbackToHeuristicOnNoKey bool
}

// DefaultSuggestLLMConfig returns the same defaults the Python loader
// applies when the user hasn't customised the section.
func DefaultSuggestLLMConfig() SuggestLLMConfig {
	return SuggestLLMConfig{
		Engine:                     "llm",
		LLMModel:                   "claude-haiku-4-5",
		LLMMaxTokens:               2000,
		AnthropicAPIKeyEnv:         "ANTHROPIC_API_KEY",
		FallbackToHeuristicOnNoKey: true,
	}
}

// SuggestLLM merges heuristic-ranked candidates with an LLM call that
// re-ranks and tightens the descriptions. Returns the augmented list
// (same Suggestion shape as the heuristic engine).
//
// On any failure (no API key, network, parse error), returns
// LLMUnavailableErr — the caller's job to fall back to the heuristic
// result.
func SuggestLLM(ctx context.Context, repoRoot string, sinceDays, minDecisions, topN int, cfg SuggestLLMConfig, now time.Time, transport LLMSuggester) ([]Suggestion, error) {
	heuristic := SuggestFromDecisions(repoRoot, sinceDays, minDecisions, topN, now)
	if len(heuristic) == 0 {
		return nil, nil
	}
	if transport == nil {
		return nil, LLMUnavailableErr
	}
	candidates := make([]HeuristicCandidate, 0, len(heuristic))
	for _, s := range heuristic {
		candidates = append(candidates, HeuristicCandidate{
			Phrase:        s.Phrase,
			Slug:          s.Slug,
			DecisionCount: s.DecisionCount,
			Evidence:      s.Evidence,
		})
	}
	req := LLMRequest{
		CandidatePatterns: candidates,
		MaxTokens:         cfg.LLMMaxTokens,
		Model:             cfg.LLMModel,
		TopN:              topN,
	}
	out, err := transport.Suggest(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		// LLM said "no clear patterns" — accept that as a real signal
		// rather than silently falling back to heuristic. Empty result
		// + nil error is the canonical "ranked nothing" outcome.
		return nil, nil
	}
	// Cap to topN even if the LLM returned more (defensive).
	if topN > 0 && len(out) > topN {
		out = out[:topN]
	}
	return out, nil
}

// AnthropicSuggester is the production LLM transport. Talks directly
// to the Anthropic Messages API (api.anthropic.com/v1/messages) via
// the standard library — no extra SDK dependency.
//
// Why net/http vs the Anthropic Go SDK: a single JSON-in / JSON-out
// round-trip doesn't need the SDK's typed surface, and shipping the
// SDK pulls in a transitively heavy dependency tree. The plan
// directives mention "Anthropic Go SDK" as an option but the actual
// API contract is small enough that net/http is cleaner here. If the
// SDK lands later (for streaming or for the count_tokens endpoint),
// drop a SDK-backed implementation behind the same LLMSuggester
// interface without touching the CLI layer.
type AnthropicSuggester struct {
	APIKey     string
	HTTPClient *http.Client
	BaseURL    string // overridable for tests
}

// NewAnthropicSuggester returns a configured suggester. Returns
// LLMUnavailableErr when the env var resolves to an empty key, so the
// CLI layer can short-circuit to the heuristic fallback before any
// network attempt.
func NewAnthropicSuggester(cfg SuggestLLMConfig) (*AnthropicSuggester, error) {
	envName := cfg.AnthropicAPIKeyEnv
	if envName == "" {
		envName = "ANTHROPIC_API_KEY"
	}
	key := os.Getenv(envName)
	if key == "" {
		return nil, fmt.Errorf("%w: %s not set", LLMUnavailableErr, envName)
	}
	return &AnthropicSuggester{
		APIKey: key,
		HTTPClient: &http.Client{
			// 30 s is generous for a single completion; past that the
			// user is better off rerunning rather than blocking the
			// CLI. Matches the cost-guardrail spirit of llm_max_tokens.
			Timeout: 30 * time.Second,
		},
		BaseURL: "https://api.anthropic.com",
	}, nil
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []anthropicMessage `json:"messages"`
	System    string             `json:"system,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []anthropicContentBlock `json:"content"`
	Error   *anthropicError         `json:"error,omitempty"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// llmJSONResponse is the shape we ask the model to emit. Keep this in
// lockstep with the system prompt below — the model needs to know the
// fields by name.
type llmJSONResponse struct {
	Patterns []llmJSONPattern `json:"patterns"`
}

type llmJSONPattern struct {
	Phrase           string `json:"phrase"`
	Slug             string `json:"slug"`
	DraftDescription string `json:"draft_description"`
}

// Suggest implements LLMSuggester. Posts the candidate set to the
// Anthropic Messages API and re-ranks based on the model's JSON
// response. Evidence + decision_count come from the candidate input
// (we never let the model invent evidence — that's an integrity gate).
func (s *AnthropicSuggester) Suggest(ctx context.Context, in LLMRequest) ([]Suggestion, error) {
	prompt := buildLLMPrompt(in)
	body, err := json.Marshal(anthropicRequest{
		Model:     in.Model,
		MaxTokens: in.MaxTokens,
		System:    llmSystemPrompt,
		Messages: []anthropicMessage{
			{Role: "user", Content: prompt},
		},
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", s.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", LLMUnavailableErr, err)
	}
	defer resp.Body.Close()
	// 1 MB ceiling on the response body. At max_tokens=2000 the
	// legitimate JSON is ~10 KB; 1 MB is plenty of headroom while
	// guarding against a misbehaving (or attacker-controlled) endpoint
	// streaming forever. The HTTPClient.Timeout caps elapsed time but
	// not bytes — LimitReader closes the gap. Per clud-bug PR #124
	// review (critical-issues-only).
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxLLMResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", LLMUnavailableErr, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: anthropic status %d", LLMUnavailableErr, resp.StatusCode)
	}
	var apiResp anthropicResponse
	if err := json.Unmarshal(raw, &apiResp); err != nil {
		return nil, fmt.Errorf("%w: parse anthropic response: %v", LLMUnavailableErr, err)
	}
	if apiResp.Error != nil {
		return nil, fmt.Errorf("%w: %s", LLMUnavailableErr, apiResp.Error.Message)
	}
	if len(apiResp.Content) == 0 {
		return nil, fmt.Errorf("%w: anthropic returned no content blocks", LLMUnavailableErr)
	}

	// Concatenate text content blocks before JSON-decoding — keeps us
	// tolerant to multi-block responses while preserving any trailing
	// commentary the model emits.
	var textBuf strings.Builder
	for _, c := range apiResp.Content {
		if c.Type == "text" {
			textBuf.WriteString(c.Text)
		}
	}
	text := textBuf.String()
	jsonBlob := extractJSONBlob(text)
	var parsed llmJSONResponse
	if err := json.Unmarshal([]byte(jsonBlob), &parsed); err != nil {
		return nil, fmt.Errorf("%w: parse llm payload: %v", LLMUnavailableErr, err)
	}

	// Re-attach evidence + decision count from the candidate set. We
	// match on slug (LLM is most reliable at echoing slugs verbatim);
	// when slug doesn't match, fall back to phrase.
	candidateBySlug := map[string]HeuristicCandidate{}
	candidateByPhrase := map[string]HeuristicCandidate{}
	for _, c := range in.CandidatePatterns {
		candidateBySlug[strings.ToLower(c.Slug)] = c
		candidateByPhrase[strings.ToLower(c.Phrase)] = c
	}

	var out []Suggestion
	for _, p := range parsed.Patterns {
		var orig HeuristicCandidate
		if c, ok := candidateBySlug[strings.ToLower(p.Slug)]; ok {
			orig = c
		} else if c, ok := candidateByPhrase[strings.ToLower(p.Phrase)]; ok {
			orig = c
		} else {
			// LLM hallucinated a slug not in the candidate set — drop
			// it so we don't pollute the output with fabricated
			// evidence.
			continue
		}
		draft := p.DraftDescription
		if strings.TrimSpace(draft) == "" {
			draft = fmt.Sprintf(
				"When working on %s, follow consistent conventions across "+
					"the codebase. (TODO: replace with concrete trigger + steps.)",
				orig.Phrase,
			)
		}
		out = append(out, Suggestion{
			Phrase:           orig.Phrase,
			Slug:             orig.Slug,
			DecisionCount:    orig.DecisionCount,
			Evidence:         orig.Evidence,
			DraftDescription: draft,
		})
	}

	// Deterministic order: preserve the LLM's ranking but break ties
	// via the heuristic's slug-sort. Stable so callers that pre-sorted
	// the candidate list keep their relative order on ties.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].DecisionCount != out[j].DecisionCount {
			return out[i].DecisionCount > out[j].DecisionCount
		}
		return strings.ToLower(out[i].Slug) < strings.ToLower(out[j].Slug)
	})
	return out, nil
}

// extractJSONBlob pulls the JSON object out of the LLM's text
// response. Models often wrap JSON in ```json fences or chat-style
// preambles; we locate the first `{` and walk forward looking for the
// matching closing `}`, honoring string-literal escapes so a `}`
// inside a quoted draft_description doesn't terminate the scan.
//
// Per clud-bug PR #124 review (minor): the previous "first `{`, last
// `}` in the whole string" heuristic broke when the model emitted a
// suffix like `"Here you go: {...} (let me know if anything's off!)"`
// — the trailing `}` outside the JSON inflated the slice and the
// parser silently failed. Brace-balanced scanning fixes that.
//
// On payloads without a balanced `{...}` returns the input verbatim
// so the JSON parser surfaces a clear error vs a silent empty result.
func extractJSONBlob(text string) string {
	first := strings.Index(text, "{")
	if first == -1 {
		return text
	}
	depth := 0
	inString := false
	escaped := false
	for i := first; i < len(text); i++ {
		c := text[i]
		switch {
		case escaped:
			// Previous char was a backslash inside a string. Skip
			// this byte regardless of what it is — that's how JSON
			// string escapes (\", \\, \n, \uXXXX) all behave for our
			// brace-tracking purposes.
			escaped = false
		case inString:
			switch c {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
		default:
			switch c {
			case '"':
				inString = true
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					return text[first : i+1]
				}
			}
		}
	}
	// No balanced closer — return verbatim so the parser fails loudly.
	return text
}

const llmSystemPrompt = `You analyse a logmind decision log and identify ` +
	`patterns that justify a new agent skill (SKILL.md). You receive a ` +
	`list of candidate token patterns (already filtered by heuristics ` +
	`from the decision log) and per-pattern evidence snippets. ` +
	`Your job is to RE-RANK the candidates and write a short ` +
	`trigger-description for each.

Return a strict JSON object with this shape — no commentary:
{"patterns": [{"phrase": "...", "slug": "...", "draft_description": "..."}, ...]}

Rules:
- Use the candidate phrases and slugs exactly as provided.
- Order patterns from most-likely-to-justify-a-skill to least.
- draft_description: one sentence, agent-facing, describing when to ` +
	`load the skill. Avoid generic phrases like "follow conventions".
- Skip candidates that are clearly not skill-worthy (one-off tooling, ` +
	`generic English words that slipped past the heuristic filter).`

func buildLLMPrompt(in LLMRequest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Candidate patterns (top %d by heuristic decision count):\n\n", len(in.CandidatePatterns))
	for i, c := range in.CandidatePatterns {
		fmt.Fprintf(&b, "%d. phrase=%q slug=%q decision_count=%d\n", i+1, c.Phrase, c.Slug, c.DecisionCount)
		for j, e := range c.Evidence {
			if j >= 3 {
				// 3 evidence snippets per candidate is enough context
				// for the LLM without ballooning token cost.
				break
			}
			fmt.Fprintf(&b, "   - %s: %s\n", e.File, e.Snippet)
		}
		fmt.Fprintln(&b)
	}
	fmt.Fprintf(&b, "Return up to %d ranked patterns in JSON.", in.TopN)
	return b.String()
}

// WriteDrafts mirrors Python's --write-drafts behaviour: each
// suggestion becomes one `skill-proposal-<slug>.md` file under the
// supplied directory. mkdir -p semantics; overwrites existing files
// (the human triggered the write, they own the directory).
func WriteDrafts(outDir string, suggestions []Suggestion) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	for _, s := range suggestions {
		path := filepath.Join(outDir, fmt.Sprintf("skill-proposal-%s.md", s.Slug))
		// atomicio.WriteFile, not os.WriteFile: "overwrites existing files"
		// means overwrite the FILE, not follow whatever a symlink at that
		// name points at. Draft names are fully derived from the slug, so a
		// hostile repo can predict them and pre-plant links.
		if err := atomicio.WriteFile(path, []byte(FormatIssueDraft(s)), 0o644); err != nil {
			return err
		}
	}
	return nil
}
