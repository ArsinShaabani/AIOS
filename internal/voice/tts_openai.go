package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// OpenAITTS uses /audio/speech.
type OpenAITTS struct {
	baseURL string
	apiKey  string
	model   string
	voice   string
	format  string
	client  *http.Client
}

var _ ports.TTS = (*OpenAITTS)(nil)

// NewOpenAITTS builds the OpenAI speech adapter.
func NewOpenAITTS(cfg config.TTSConfig) *OpenAITTS {
	timeout := cfg.Timeout.Std()
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	model := cfg.Model
	if model == "" {
		model = "gpt-4o-mini-tts"
	}
	voice := cfg.Voice
	if voice == "" {
		voice = "alloy"
	}
	format := cfg.Format
	if format == "" {
		format = "wav"
	}
	return &OpenAITTS{baseURL: base, apiKey: cfg.APIKey, model: model, voice: voice, format: format, client: &http.Client{Timeout: timeout}}
}

func (o *OpenAITTS) Name() string { return "openai" }

// Synthesize calls /audio/speech.
func (o *OpenAITTS) Synthesize(ctx context.Context, text string, opts ports.VoiceOptions) (ports.Audio, error) {
	payload, _ := json.Marshal(map[string]any{
		"model": o.model, "voice": firstNonEmpty(opts.Voice, o.voice),
		"input": text, "response_format": firstNonEmpty(opts.Format, o.format),
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/audio/speech", bytes.NewReader(payload))
	if err != nil {
		return ports.Audio{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)
	resp, err := o.client.Do(req)
	if err != nil {
		return ports.Audio{}, platform.Wrap(platform.KindUnavailable, "voice.tts", "OpenAI TTS unreachable", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return ports.Audio{}, platform.Wrap(platform.KindUnavailable, "voice.tts", platform.Truncate(string(raw), 300), nil)
	}
	return readAudio(resp, o.format)
}

// MockTTS generates a deterministic tone so the voice loop is fully testable.
type MockTTS struct{}

var _ ports.TTS = (*MockTTS)(nil)

func (MockTTS) Name() string { return "mock" }

func (MockTTS) Synthesize(_ context.Context, text string, opts ports.VoiceOptions) (ports.Audio, error) {
	ms := 300 + len([]rune(text))*8
	if ms > 1500 {
		ms = 1500
	}
	if ms < 250 {
		ms = 250
	}
	rate := 16000
	wav := EncodeWAV(Beep(ms, rate, 520), rate)
	return ports.Audio{Data: wav, Format: "wav", SampleRate: rate, DurationMS: int64(ms)}, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
