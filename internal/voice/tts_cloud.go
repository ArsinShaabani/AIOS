package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// CoquiTTS talks to a Coqui TTS server (the common /api/tts endpoint used by
// `tts-server` and the Coqui studio container).
type CoquiTTS struct {
	baseURL  string
	voice    string
	language string
	format   string
	client   *http.Client
}

var _ ports.TTS = (*CoquiTTS)(nil)

// NewCoquiTTS builds a Coqui TTS adapter.
func NewCoquiTTS(cfg config.TTSConfig) *CoquiTTS {
	timeout := cfg.Timeout.Std()
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	format := cfg.Format
	if format == "" {
		format = "wav"
	}
	return &CoquiTTS{
		baseURL:  strings.TrimRight(cfg.BaseURL, "/"),
		voice:    cfg.Voice,
		language: cfg.Language,
		format:   format,
		client:   &http.Client{Timeout: timeout},
	}
}

func (t *CoquiTTS) Name() string { return "coqui_http" }

// Synthesize renders text with the configured speaker.
func (t *CoquiTTS) Synthesize(ctx context.Context, text string, opts ports.VoiceOptions) (ports.Audio, error) {
	if strings.TrimSpace(text) == "" {
		return ports.Audio{}, platform.Invalid("voice.empty_text", "text is required")
	}
	voice := firstNonEmpty(opts.Voice, t.voice)
	language := firstNonEmpty(t.language, "en")
	body := map[string]any{"text": text, "speaker_id": voice, "language_id": language}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+"/api/tts", bytes.NewReader(payload))
	if err != nil {
		return ports.Audio{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "audio/*")
	resp, err := t.client.Do(req)
	if err != nil {
		return ports.Audio{}, platform.Wrap(platform.KindUnavailable, "voice.tts", "Coqui TTS unreachable", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return ports.Audio{}, platform.Wrap(platform.KindUnavailable, "voice.tts",
			fmt.Sprintf("Coqui TTS returned %d: %s", resp.StatusCode, platform.Truncate(string(raw), 160)), nil)
	}
	return readAudio(resp, t.format)
}

// AzureTTS uses the Azure Speech REST API. No SDK and no cgo: a single SSML POST.
type AzureTTS struct {
	endpoint string
	apiKey   string
	voice    string
	format   string
	client   *http.Client
}

var _ ports.TTS = (*AzureTTS)(nil)

// NewAzureTTS builds an Azure Speech adapter.
func NewAzureTTS(cfg config.TTSConfig) *AzureTTS {
	endpoint := strings.TrimRight(cfg.BaseURL, "/")
	if endpoint == "" && cfg.Region != "" {
		endpoint = "https://" + cfg.Region + ".tts.speech.microsoft.com/cognitiveservices/v1"
	}
	timeout := cfg.Timeout.Std()
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	format := cfg.Format
	if format == "" {
		format = "wav"
	}
	return &AzureTTS{endpoint: endpoint, apiKey: cfg.APIKey, voice: cfg.Voice,
		format: format, client: &http.Client{Timeout: timeout}}
}

func (t *AzureTTS) Name() string { return "azure" }

// Synthesize renders SSML through Azure Speech.
func (t *AzureTTS) Synthesize(ctx context.Context, text string, opts ports.VoiceOptions) (ports.Audio, error) {
	if strings.TrimSpace(text) == "" {
		return ports.Audio{}, platform.Invalid("voice.empty_text", "text is required")
	}
	if t.endpoint == "" {
		return ports.Audio{}, platform.Invalid("voice.azure_endpoint", "voice.tts.base_url or voice.tts.region is required for azure")
	}
	if t.apiKey == "" {
		return ports.Audio{}, platform.Invalid("voice.azure_key", "voice.tts.api_key is required for azure")
	}
	voice := firstNonEmpty(opts.Voice, t.voice, "en-US-JennyNeural")
	output := azureOutputFormat(t.format)
	ssml := `<speak version="1.0" xmlns="http://www.w3.org/2001/10/synthesis" xml:lang="en-US"><voice name="` +
		html.EscapeString(voice) + `">` + html.EscapeString(text) + `</voice></speak>`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, strings.NewReader(ssml))
	if err != nil {
		return ports.Audio{}, err
	}
	req.Header.Set("Ocp-Apim-Subscription-Key", t.apiKey)
	req.Header.Set("Content-Type", "application/ssml+xml")
	req.Header.Set("X-Microsoft-OutputFormat", output)
	req.Header.Set("User-Agent", "AIOS")
	resp, err := t.client.Do(req)
	if err != nil {
		return ports.Audio{}, platform.Wrap(platform.KindUnavailable, "voice.tts", "Azure TTS unreachable", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return ports.Audio{}, platform.Wrap(platform.KindUnavailable, "voice.tts",
			fmt.Sprintf("Azure TTS returned %d: %s", resp.StatusCode, platform.Truncate(string(raw), 160)), nil)
	}
	return readAudio(resp, t.format)
}

func azureOutputFormat(format string) string {
	switch format {
	case "mp3":
		return "audio-24khz-48kbitrate-mono-mp3"
	case "opus":
		return "ogg-24khz-16bit-mono-opus"
	default:
		return "riff-24khz-16bit-mono-pcm"
	}
}
