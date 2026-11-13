package voice

import (
	"context"
	"log/slog"
	"regexp"
	"strings"
	"unicode"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

var urlPattern = regexp.MustCompile(`https?://\S+`)

// Service exposes speech-to-text, text-to-speech and wake-word handling.
type Service struct {
	cfg    config.VoiceConfig
	stt    ports.STT
	tts    ports.TTS
	silero *SileroHTTP
	log    *slog.Logger
}

// NewService builds the voice service.
func NewService(cfg config.VoiceConfig, stt ports.STT, tts ports.TTS, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{cfg: cfg, stt: stt, tts: tts, log: log}
}

// Enabled reports whether voice is configured.
func (s *Service) Enabled() bool { return s != nil && s.cfg.Enabled && s.stt != nil && s.tts != nil }

// STT returns the configured recogniser.
func (s *Service) STT() ports.STT { return s.stt }

// TTS returns the configured synthesiser.
func (s *Service) TTS() ports.TTS { return s.tts }

// Config exposes the voice configuration.
func (s *Service) Config() config.VoiceConfig { return s.cfg }

// Transcribe converts audio to text using the configured language.
func (s *Service) Transcribe(ctx context.Context, audio ports.Audio) (ports.Transcript, error) {
	return s.TranscribeLanguage(ctx, audio, s.cfg.STT.Language)
}

// TranscribeLanguage converts audio to text with an explicit language override
// (per-user voice profile).
func (s *Service) TranscribeLanguage(ctx context.Context, audio ports.Audio, language string) (ports.Transcript, error) {
	if s.stt == nil {
		return ports.Transcript{}, platform.Unavailable("voice.disabled", "voice is disabled")
	}
	if language == "" {
		language = s.cfg.STT.Language
	}
	tr, err := s.stt.Transcribe(ctx, audio, language)
	if err != nil {
		return tr, err
	}
	tr.Text = strings.TrimSpace(tr.Text)
	tr.IsFinal = true
	if tr.DurationMS == 0 {
		tr.DurationMS = audio.DurationMS
	}
	return tr, nil
}

// Synthesize converts a reply into speakable audio using deployment defaults.
func (s *Service) Synthesize(ctx context.Context, text string) (ports.Audio, error) {
	return s.SynthesizeOptions(ctx, text, ports.VoiceOptions{
		Voice: s.cfg.TTS.Voice, Format: s.cfg.TTS.Format, Speed: s.cfg.TTS.Speed,
	})
}

// SynthesizeOptions converts a reply using an explicit voice profile. Markdown is
// stripped because reading asterisks and code fences aloud is unpleasant.
func (s *Service) SynthesizeOptions(ctx context.Context, text string, opts ports.VoiceOptions) (ports.Audio, error) {
	if s.tts == nil {
		return ports.Audio{}, platform.Unavailable("voice.disabled", "voice is disabled")
	}
	speakable := Speakable(text)
	if speakable == "" {
		speakable = "I don't have anything to say."
	}
	if opts.Voice == "" {
		opts.Voice = s.cfg.TTS.Voice
	}
	if opts.Format == "" {
		opts.Format = s.cfg.TTS.Format
	}
	if opts.Speed <= 0 {
		opts.Speed = s.cfg.TTS.Speed
	}
	return s.tts.Synthesize(ctx, speakable, opts)
}

// WakeWord checks a transcript against the configured wake words. When wake
// words are disabled every utterance activates. It returns the command with the
// wake word removed.
func (s *Service) WakeWord(text string) (bool, string) {
	if !s.cfg.WakeWord.Enabled || len(s.cfg.WakeWord.Words) == 0 {
		return true, strings.TrimSpace(text)
	}
	lower := strings.ToLower(text)
	for _, word := range s.cfg.WakeWord.Words {
		word = strings.ToLower(strings.TrimSpace(word))
		if word == "" {
			continue
		}
		if idx := strings.Index(lower, word); idx >= 0 {
			rest := text[idx+len(word):]
			return true, strings.TrimSpace(strings.TrimLeft(rest, " ,.!?؟،"))
		}
	}
	return false, ""
}

// BuildEngines creates STT/TTS adapters from configuration.
func BuildEngines(cfg config.VoiceConfig) (ports.STT, ports.TTS, error) {
	sttCfg := cfg.STT
	switch cfg.STT.Provider {
	case "openai":
		if sttCfg.BaseURL == "" {
			sttCfg.BaseURL = "https://api.openai.com/v1"
		}
		if sttCfg.APIKey == "" {
			return nil, nil, platform.Invalid("voice.openai_key", "voice.stt.api_key is required for the OpenAI provider")
		}
	case "whisper_http":
		if sttCfg.BaseURL == "" {
			sttCfg.BaseURL = "http://127.0.0.1:8000/v1"
		}
	case "mock":
	default:
		return nil, nil, platform.Invalid("voice.stt_provider", "unknown STT provider: "+cfg.STT.Provider)
	}

	ttsCfg := cfg.TTS
	switch cfg.TTS.Provider {
	case "openai":
		if ttsCfg.APIKey == "" {
			return nil, nil, platform.Invalid("voice.openai_tts_key", "voice.tts.api_key is required for the OpenAI provider")
		}
	case "piper_http", "edge_http":
		if ttsCfg.BaseURL == "" {
			return nil, nil, platform.Invalid("voice.tts_url", "voice.tts.base_url is required for "+cfg.TTS.Provider)
		}
	case "mock":
	default:
		return nil, nil, platform.Invalid("voice.tts_provider", "unknown TTS provider: "+cfg.TTS.Provider)
	}

	var stt ports.STT
	switch cfg.STT.Provider {
	case "mock":
		stt = MockSTT{}
	default:
		stt = NewWhisperSTT(sttCfg)
	}
	var tts ports.TTS
	switch cfg.TTS.Provider {
	case "mock":
		tts = MockTTS{}
	case "piper_http":
		tts = NewPiperTTS(ttsCfg)
	case "edge_http":
		tts = NewEdgeTTS(ttsCfg)
	case "openai":
		tts = NewOpenAITTS(ttsCfg)
	}
	return stt, tts, nil
}

// Speakable removes markup, URLs and code fences from a reply.
func Speakable(text string) string {
	s := text
	for {
		start := strings.Index(s, "```")
		if start < 0 {
			break
		}
		end := strings.Index(s[start+3:], "```")
		if end < 0 {
			s = s[:start] + " (code omitted) "
			break
		}
		s = s[:start] + " (code omitted) " + s[start+3+end+3:]
	}
	s = urlPattern.ReplaceAllString(s, " link ")
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '#', '*', '_', '`', '>', '|', '~':
			b.WriteRune(' ')
		case '\n':
			b.WriteString(". ")
		default:
			b.WriteRune(r)
		}
	}
	out := strings.Join(strings.Fields(b.String()), " ")
	out = strings.ReplaceAll(out, " .", ".")
	out = strings.ReplaceAll(out, "..", ".")
	return platform.Truncate(strings.TrimSpace(out), 1200)
}

// SplitSentences splits a reply into speech-sized chunks for streaming TTS.
func SplitSentences(text string, maxChars int) []string {
	if maxChars <= 0 {
		maxChars = 220
	}
	var (
		out  []string
		cur  strings.Builder
		last rune
	)
	flush := func() {
		chunk := strings.TrimSpace(cur.String())
		if chunk != "" {
			out = append(out, chunk)
		}
		cur.Reset()
	}
	for _, r := range text {
		cur.WriteRune(r)
		if r == '.' || r == '!' || r == '?' || r == '؟' || r == '۔' || r == '\n' {
			if cur.Len() >= 40 {
				flush()
			}
		} else if cur.Len() >= maxChars && (unicode.IsSpace(r) || r == '،') {
			flush()
		} else if cur.Len() >= maxChars {
			// Hard-split scripts without spaces so a single long token cannot
			// exceed the TTS request limit.
			flush()
		}
		last = r
	}
	_ = last
	flush()
	return out
}
