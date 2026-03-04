package ai

type Song struct {
	Title  string `json:"title"`
	Artist string `json:"artist"`
}

type APIKeys struct {
	Anthropic string
	OpenAI    string
	Google    string
	XAI       string
}

const (
	DefaultClaudeModel = "claude-sonnet-4-6"
	DefaultOpenAIModel = "gpt-5.2"
	DefaultGeminiModel = "gemini-3-flash-preview"
	DefaultGrokModel   = "grok-4-1-fast-reasoning"
)

type Models struct {
	Claude string
	OpenAI string
	Gemini string
	Grok   string
}

func (m Models) WithDefaults() Models {
	if m.Claude == "" {
		m.Claude = DefaultClaudeModel
	}
	if m.OpenAI == "" {
		m.OpenAI = DefaultOpenAIModel
	}
	if m.Gemini == "" {
		m.Gemini = DefaultGeminiModel
	}
	if m.Grok == "" {
		m.Grok = DefaultGrokModel
	}
	return m
}

type RankedSong struct {
	Song
	Votes int `json:"votes"`
}
