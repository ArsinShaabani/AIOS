package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// HTTPTTS covers Piper's HTTP wrapper, Edge-TTS bridges and any service that
// accepts {text, voice, speed} and returns audio bytes.
type HTTPTTS struct {
	name    string
	baseURL string
	apiKey  string
	voice   string
	format  string
	speed   float64
	path    string
	client  *http.Client
}

var _ ports.TTS = (*HTTPTTS)(nil)

// NewPiperTTS builds a Piper HTTP adapter.
func NewPiperTTS(cfg config.TTSConfig) *HTTPTTS {
	t := newHTTPTTS(cfg)
	t.name = "piper_http"
	t.path = "/api/tts"
	return t
}

// NewEdgeTTS builds an Edge-TTS bridge adapter.
func NewEdgeTTS(cfg config.TTSConfig) *HTTPTTS {
	t := newHTTPTTS(cfg)
	t.name = "edge_http"
	t.path = "/tts"
	return t
}

func newHTTPTTS(cfg config.TTSConfig) *HTTPTTS {
	timeout := cfg.Timeout.Std()
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	format := cfg.Format
	if format == "" {
		format = "wav"
	}
	speed := cfg.Speed
	if speed <= 0 {
		speed = 1
	}
	return &HTTPTTS{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"), apiKey: cfg.APIKey,
		voice: cfg.Voice, format: format, speed: speed,
		client: &http.Client{Timeout: timeout},
	}
}

func (t *HTTPTTS) Name() string { return t.name }

// Synthesize requests speech audio.
func (t *HTTPTTS) Synthesize(ctx context.Context, text string, opts ports.VoiceOptions) (ports.Audio, error) {
	if strings.TrimSpace(text) == "" {
		return ports.Audio{}, platform.Invalid("voice.empty_text", "text is required")
	}
	voice := firstNonEmpty(opts.Voice, t.voice)
	format := firstNonEmpty(opts.Format, t.format)
	speed := opts.Speed
	if speed <= 0 {
		speed = t.speed
	}
	body := map[string]any{"text": text, "voice": voice, "format": format, "speed": speed}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+t.path, bytes.NewReader(payload))
	if err != nil {
		return ports.Audio{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "audio/*")
	if t.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+t.apiKey)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return ports.Audio{}, platform.Wrap(platform.KindUnavailable, "voice.tts", "TTS service unreachable", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return t.synthesizeAt(ctx, text, voice, format, speed, "/")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return ports.Audio{}, platform.Wrap(platform.KindUnavailable, "voice.tts",
			fmt.Sprintf("TTS service returned %d: %s", resp.StatusCode, platform.Truncate(string(raw), 200)), nil)
	}
	return readAudio(resp, format)
}

func (t *HTTPTTS) synthesizeAt(ctx context.Context, text, voice, format string, speed float64, path string) (ports.Audio, error) {
	payload, _ := json.Marshal(map[string]any{"text": text, "voice": voice, "format": format, "speed": speed})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return ports.Audio{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return ports.Audio{}, platform.Wrap(platform.KindUnavailable, "voice.tts", "TTS service unreachable", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ports.Audio{}, platform.Wrap(platform.KindUnavailable, "voice.tts", fmt.Sprintf("TTS service returned %d", resp.StatusCode), nil)
	}
	return readAudio(resp, format)
}

func readAudio(resp *http.Response, format string) (ports.Audio, error) {
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return ports.Audio{}, platform.Wrap(platform.KindUnavailable, "voice.tts_read", "cannot read audio", err)
	}
	if len(data) == 0 {
		return ports.Audio{}, platform.Unavailable("voice.tts_empty", "TTS service returned no audio")
	}
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "mpeg") || strings.Contains(contentType, "mp3") {
		format = "mp3"
	}
	return ports.Audio{Data: data, Format: format}, nil
}
