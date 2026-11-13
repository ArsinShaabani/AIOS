package voice

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// AudioWakeEnabled reports whether the configured wake word runs on audio
// (openWakeWord) rather than on the transcript.
func (s *Service) AudioWakeEnabled() bool {
	return s != nil && s.cfg.WakeWord.Enabled && s.cfg.WakeWord.Provider == "openwakeword_http"
}

// DetectWakeWord scores an utterance with openWakeWord. The bridge contract is a
// POST of {"audio": "<base64 wav>", "words": [...]} returning
// {"score": 0.0..1.0, "word": "hey aios"}.
func (s *Service) DetectWakeWord(ctx context.Context, audio ports.Audio) (bool, error) {
	if !s.AudioWakeEnabled() {
		return false, nil
	}
	endpoint := strings.TrimSpace(s.cfg.WakeWord.Endpoint)
	if endpoint == "" {
		return false, platform.Invalid("voice.wake_endpoint", "voice.wake_word.endpoint is not configured")
	}
	if len(audio.Data) == 0 {
		return false, platform.Invalid("voice.wake_audio", "audio is required for wake-word detection")
	}
	data := audio.Data
	if audio.Format == "pcm16" {
		rate := audio.SampleRate
		if rate <= 0 {
			rate = s.cfg.SampleRate
		}
		data = EncodeWAV(BytesToPCM16(data), rate)
	}
	timeout := s.cfg.WakeWord.Timeout.Std()
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	client := &http.Client{Timeout: timeout}
	payload, err := json.Marshal(map[string]any{
		"audio": base64.StdEncoding.EncodeToString(data),
		"words": s.cfg.WakeWord.Words,
	})
	if err != nil {
		return false, platform.Wrap(platform.KindInternal, "voice.wake_encode", "cannot encode wake request", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return false, platform.Wrap(platform.KindInvalid, "voice.wake_request", "invalid wake endpoint", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return false, platform.Wrap(platform.KindUnavailable, "voice.wake_unreachable", "wake-word service unreachable", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return false, platform.Wrap(platform.KindUnavailable, "voice.wake_read", "cannot read wake response", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, platform.Wrap(platform.KindUnavailable, "voice.wake_status",
			fmt.Sprintf("wake-word service returned %d", resp.StatusCode), nil)
	}
	var out struct {
		Score float64 `json:"score"`
		Word  string  `json:"word"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return false, platform.Wrap(platform.KindUnavailable, "voice.wake_parse", "invalid wake-word response", err)
	}
	threshold := s.cfg.WakeWord.Threshold
	if threshold <= 0 {
		threshold = 0.5
	}
	return out.Score >= threshold, nil
}

// StripWakeWord removes configured wake words from a transcript, regardless of
// position, and trims leading punctuation.
func (s *Service) StripWakeWord(text string) string {
	out := text
	for _, word := range s.cfg.WakeWord.Words {
		word = strings.ToLower(strings.TrimSpace(word))
		if word == "" {
			continue
		}
		for {
			idx := strings.Index(strings.ToLower(out), word)
			if idx < 0 {
				break
			}
			out = out[:idx] + out[idx+len(word):]
		}
	}
	return strings.TrimSpace(strings.TrimLeft(out, " ,.!?؟،"))
}
