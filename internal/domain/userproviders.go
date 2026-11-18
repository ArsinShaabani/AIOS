package domain

import "time"

// UserProvider is a BYOK (bring your own key) AI service configured on a single
// user account. The API key is stored encrypted and never returned by the API.
type UserProvider struct {
	ID            string    `json:"id"`
	UserID        string    `json:"user_id"`
	Name          string    `json:"name"`
	Kind          string    `json:"kind"` // openai | anthropic | gemini | ollama
	BaseURL       string    `json:"base_url"`
	DefaultModel  string    `json:"default_model"`
	Models        []string  `json:"models"`
	APIKeyEnc     string    `json:"-"`
	Enabled       bool      `json:"enabled"`
	MaxTokens     int       `json:"max_tokens"`
	ContextWindow int       `json:"context_window"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
