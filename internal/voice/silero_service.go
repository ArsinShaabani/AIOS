package voice

import (
	"context"

	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// Detector returns the neural speech detector when configured.
func (s *Service) Silero() *SileroHTTP {
	if s == nil || s.cfg.VAD.Provider != "silero_http" {
		return nil
	}
	if s.silero == nil {
		s.silero = NewSilero(s.cfg.VAD, s.cfg.SampleRate)
	}
	return s.silero
}

// ConfirmSpeech double-checks an utterance with Silero when configured. A bridge
// failure falls back to accepting the energy VAD's decision (fail-open), because
// losing a turn is worse than transcribing a noisy one.
func (s *Service) ConfirmSpeech(ctx context.Context, audio ports.Audio) bool {
	detector := s.Silero()
	if detector == nil {
		return true
	}
	ok, err := detector.ConfirmsSpeech(ctx, audio)
	if err != nil {
		platform.LoggerFrom(ctx).Warn("silero VAD unavailable, keeping energy decision", "error", err)
		return true
	}
	return ok
}
