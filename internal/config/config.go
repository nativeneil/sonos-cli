package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
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
