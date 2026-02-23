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

type RankedSong struct {
	Song
	Votes int `json:"votes"`
}
