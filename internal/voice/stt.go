package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// WhisperSTT calls an OpenAI-compatible /audio/transcriptions endpoint. It works
// with the OpenAI API and with faster-whisper / whisper.cpp HTTP servers.
type WhisperSTT struct {
	name    string
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

var _ ports.STT = (*WhisperSTT)(nil)

// NewWhisperSTT builds the STT adapter.
func NewWhisperSTT(cfg config.STTConfig) *WhisperSTT {
	timeout := cfg.Timeout.Std()
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	model := cfg.Model
	if model == "" {
		model = "whisper-1"
	}
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		base = "http://127.0.0.1:8000/v1"
	}
	return &WhisperSTT{name: "whisper_http", baseURL: base, apiKey: cfg.APIKey, model: model, client: &http.Client{Timeout: timeout}}
}

func (w *WhisperSTT) Name() string { return w.name }

// Transcribe uploads WAV audio and returns the transcript.
func (w *WhisperSTT) Transcribe(ctx context.Context, audio ports.Audio, language string) (ports.Transcript, error) {
	if len(audio.Data) == 0 {
		return ports.Transcript{}, platform.Invalid("voice.empty_audio", "audio is empty")
	}
	data := audio.Data
	if audio.Format == "pcm16" {
		rate := audio.SampleRate
		if rate <= 0 {
			rate = 16000
		}
		data = EncodeWAV(BytesToPCM16(data), rate)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "audio.wav")
	if err != nil {
		return ports.Transcript{}, platform.Wrap(platform.KindInternal, "voice.multipart", "cannot build upload", err)
	}
	if _, err := part.Write(data); err != nil {
		return ports.Transcript{}, err
	}
	_ = writer.WriteField("model", w.model)
	_ = writer.WriteField("response_format", "json")
	_ = writer.WriteField("temperature", "0")
	if language != "" {
		_ = writer.WriteField("language", language)
	}
	if err := writer.Close(); err != nil {
		return ports.Transcript{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.baseURL+"/audio/transcriptions", &body)
	if err != nil {
		return ports.Transcript{}, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if w.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+w.apiKey)
	}
	resp, err := w.client.Do(req)
	if err != nil {
		return ports.Transcript{}, platform.Wrap(platform.KindUnavailable, "voice.stt", "speech service unreachable", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ports.Transcript{}, platform.Wrap(platform.KindUnavailable, "voice.stt", fmt.Sprintf("speech service returned %d: %s", resp.StatusCode, platform.Truncate(string(raw), 200)), nil)
	}
	var payload struct {
		Text     string  `json:"text"`
		Language string  `json:"language"`
		Duration float64 `json:"duration"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ports.Transcript{}, platform.Wrap(platform.KindUnavailable, "voice.stt_parse", "invalid transcription response", err)
	}
	return ports.Transcript{
		Text: strings.TrimSpace(payload.Text), Language: payload.Language,
		DurationMS: int64(payload.Duration * 1000), IsFinal: true,
	}, nil
}

// MockSTT is used in tests and in the demo profile.
type MockSTT struct{}

var _ ports.STT = (*MockSTT)(nil)

func (MockSTT) Name() string { return "mock" }

func (MockSTT) Transcribe(_ context.Context, audio ports.Audio, language string) (ports.Transcript, error) {
	if len(audio.Data) == 0 {
		return ports.Transcript{}, platform.Invalid("voice.empty_audio", "audio is empty")
	}
	if language == "" {
		language = "en"
	}
	return ports.Transcript{Text: "hello aios", Language: language, IsFinal: true, DurationMS: audio.DurationMS}, nil
}
