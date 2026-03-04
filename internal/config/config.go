package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"

	"sonos-playlist/internal/ai"
)

type Provider string

const (
	ProviderClaude Provider = "claude"
	ProviderOpenAI Provider = "openai"
	ProviderGemini Provider = "gemini"
	ProviderGrok   Provider = "grok"
)

type Config struct {
	AnthropicAPIKey string
	OpenAIAPIKey    string
	GoogleAPIKey    string
	XAIAPIKey       string
	ClaudeModel     string
	OpenAIModel     string
	GeminiModel     string
	GrokModel       string
	SonosAPIURL     string
	DefaultRoom     string
	DefaultProvider Provider
	DefaultCount    int
}

type fileConfig struct {
	SonosAPIURL     string   `json:"sonosApiUrl"`
	DefaultRoom     string   `json:"defaultRoom"`
	DefaultProvider Provider `json:"defaultProvider"`
	DefaultCount    int      `json:"defaultCount"`
	ClaudeModel     string   `json:"claudeModel"`
	OpenAIModel     string   `json:"openaiModel"`
	GeminiModel     string   `json:"geminiModel"`
	GrokModel       string   `json:"grokModel"`
}

func init() {
	_ = godotenv.Load()
}

func Load() Config {
	fc := loadFileConfig()

	sonosURL := firstNonEmpty(os.Getenv("SONOS_API_URL"), fc.SonosAPIURL, "http://localhost:5005")
	defaultRoom := firstNonEmpty(os.Getenv("SONOS_DEFAULT_ROOM"), fc.DefaultRoom)

	provider := fc.DefaultProvider
	if provider == "" {
		provider = ProviderClaude
	}

	count := fc.DefaultCount
	if count == 0 {
		count = 15
	}

	return Config{
		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
		OpenAIAPIKey:    os.Getenv("OPENAI_API_KEY"),
		GoogleAPIKey:    os.Getenv("GOOGLE_API_KEY"),
		XAIAPIKey:       firstNonEmpty(os.Getenv("XAI_API_KEY"), os.Getenv("GROK_API_KEY")),
		ClaudeModel:     firstNonEmpty(os.Getenv("ANTHROPIC_MODEL"), os.Getenv("SONOS_CLAUDE_MODEL"), fc.ClaudeModel, ai.DefaultClaudeModel),
		OpenAIModel:     firstNonEmpty(os.Getenv("OPENAI_MODEL"), os.Getenv("SONOS_OPENAI_MODEL"), fc.OpenAIModel, ai.DefaultOpenAIModel),
		GeminiModel:     firstNonEmpty(os.Getenv("GOOGLE_MODEL"), os.Getenv("GEMINI_MODEL"), os.Getenv("SONOS_GEMINI_MODEL"), fc.GeminiModel, ai.DefaultGeminiModel),
		GrokModel:       firstNonEmpty(os.Getenv("XAI_MODEL"), os.Getenv("GROK_MODEL"), os.Getenv("SONOS_GROK_MODEL"), fc.GrokModel, ai.DefaultGrokModel),
		SonosAPIURL:     sonosURL,
		DefaultRoom:     defaultRoom,
		DefaultProvider: provider,
		DefaultCount:    count,
	}
}

func loadFileConfig() fileConfig {
	home, err := os.UserHomeDir()
	if err != nil {
		return fileConfig{}
	}
	configPath := filepath.Join(home, ".config", "sonos-playlist", "config.json")
	b, err := os.ReadFile(configPath)
	if err != nil {
		return fileConfig{}
	}
	var fc fileConfig
	if err := json.Unmarshal(b, &fc); err != nil {
		return fileConfig{}
	}
	return fc
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
