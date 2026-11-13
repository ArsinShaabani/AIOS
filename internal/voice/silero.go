package voice

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// SpeechDetector judges a finished utterance. The built-in energy VAD is used
// while audio is streaming; a neural detector (Silero) upgrades the final
// decision without adding cgo or ONNX to the core.
type SpeechDetector interface {
	Speech(ctx context.Context, audio ports.Audio) (float64, error)
}

// SileroHTTP scores audio with a Silero VAD HTTP bridge:
//
//	POST endpoint {"audio":"<base64 wav>","sample_rate":16000}
//	→ {"speech": 0.0..1.0}
type SileroHTTP struct {
	endpoint   string
	threshold  float64
	sampleRate int
	client     *http.Client
}

var _ SpeechDetector = (*SileroHTTP)(nil)

// NewSilero builds the bridge client.
func NewSilero(cfg config.VADConfig, sampleRate int) *SileroHTTP {
	timeout := cfg.Timeout.Std()
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	if sampleRate <= 0 {
		sampleRate = 16000
	}
	threshold := cfg.Threshold
	if threshold <= 0 {
		threshold = 0.5
	}
	return &SileroHTTP{
		endpoint: cfg.Endpoint, threshold: threshold, sampleRate: sampleRate,
		client: &http.Client{Timeout: timeout},
	}
}

// Speech returns the probability that the utterance contains speech.
func (s *SileroHTTP) Speech(ctx context.Context, audio ports.Audio) (float64, error) {
	if s.endpoint == "" {
		return 0, platform.Invalid("voice.vad_endpoint", "voice.vad.endpoint is not configured")
	}
	if len(audio.Data) == 0 {
		return 0, platform.Invalid("voice.vad_audio", "audio is required for speech detection")
	}
	data := audio.Data
	rate := audio.SampleRate
	if rate <= 0 {
		rate = s.sampleRate
	}
	if audio.Format == "pcm16" {
		data = EncodeWAV(BytesToPCM16(data), rate)
	}
	payload, err := json.Marshal(map[string]any{
		"audio": base64.StdEncoding.EncodeToString(data), "sample_rate": rate,
	})
	if err != nil {
		return 0, platform.Wrap(platform.KindInternal, "voice.vad_encode", "cannot encode VAD request", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(payload))
	if err != nil {
		return 0, platform.Wrap(platform.KindInvalid, "voice.vad_request", "invalid VAD endpoint", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return 0, platform.Wrap(platform.KindUnavailable, "voice.vad_unreachable", "VAD service unreachable", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return 0, platform.Wrap(platform.KindUnavailable, "voice.vad_read", "cannot read VAD response", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, platform.Wrap(platform.KindUnavailable, "voice.vad_status",
			fmt.Sprintf("VAD service returned %d", resp.StatusCode), nil)
	}
	var out struct {
		Speech float64 `json:"speech"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return 0, platform.Wrap(platform.KindUnavailable, "voice.vad_parse", "invalid VAD response", err)
	}
	if out.Speech < 0 {
		out.Speech = 0
	}
	if out.Speech > 1 {
		out.Speech = 1
	}
	return out.Speech, nil
}

// Threshold exposes the configured activation probability.
func (s *SileroHTTP) Threshold() float64 { return s.threshold }

// ConfirmsSpeech reports whether the detector agrees the utterance is speech.
// Errors are returned so callers can choose fail-open or fail-closed.
func (s *SileroHTTP) ConfirmsSpeech(ctx context.Context, audio ports.Audio) (bool, error) {
	score, err := s.Speech(ctx, audio)
	if err != nil {
		return false, err
	}
	return score >= s.threshold, nil
}
