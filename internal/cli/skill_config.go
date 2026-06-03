package cli

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/thrillmade/logmind/internal/skill"
)

// userConfigShape is the slice of .logmind/config.yml the suggest
// command cares about. Defined locally (vs in internal/config) so the
// B5 wave doesn't fight the parallel B6 rewrite of the config loader.
// Once B6 lands a richer Config struct, this wrapper drops and the
// command calls config.Load(cwd).SkillSuggest directly.
type userConfigShape struct {
	SkillSuggest struct {
		Engine                     *string `yaml:"engine,omitempty"`
		LLMModel                   *string `yaml:"llm_model,omitempty"`
		LLMMaxTokens               *int    `yaml:"llm_max_tokens,omitempty"`
		AnthropicAPIKeyEnv         *string `yaml:"anthropic_api_key_env,omitempty"`
		FallbackToHeuristicOnNoKey *bool   `yaml:"fallback_to_heuristic_on_no_key,omitempty"`
	} `yaml:"skill_suggest"`
}

// mergeUserSuggestConfig overlays the `skill_suggest:` block from
// .logmind/config.yml (when present) onto an in-memory default config.
//
// Missing keys keep the default; missing file leaves cfg untouched.
// Parse errors silently fall through — matching Python's
// "broken config = user fixing it later" stance.
func mergeUserSuggestConfig(cwd string, cfg *skill.SuggestLLMConfig) {
	if cfg == nil {
		return
	}
	path := filepath.Join(cwd, ".logmind", "config.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var u userConfigShape
	if err := yaml.Unmarshal(data, &u); err != nil {
		return
	}
	if u.SkillSuggest.Engine != nil {
		cfg.Engine = *u.SkillSuggest.Engine
	}
	if u.SkillSuggest.LLMModel != nil {
		cfg.LLMModel = *u.SkillSuggest.LLMModel
	}
	if u.SkillSuggest.LLMMaxTokens != nil {
		cfg.LLMMaxTokens = *u.SkillSuggest.LLMMaxTokens
	}
	if u.SkillSuggest.AnthropicAPIKeyEnv != nil {
		cfg.AnthropicAPIKeyEnv = *u.SkillSuggest.AnthropicAPIKeyEnv
	}
	if u.SkillSuggest.FallbackToHeuristicOnNoKey != nil {
		cfg.FallbackToHeuristicOnNoKey = *u.SkillSuggest.FallbackToHeuristicOnNoKey
	}
}
