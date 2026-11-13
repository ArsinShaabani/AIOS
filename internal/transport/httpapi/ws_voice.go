package httpapi

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/ArsinShaabani/AIOS/internal/brain"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
	"github.com/ArsinShaabani/AIOS/internal/voice"
)

type voiceCommand struct {
	Type          string `json:"type"`
	Message       string `json:"message"`
	SessionID     string `json:"session_id"`
	AgentID       string `json:"agent_id"`
	PersonalityID string `json:"personality_id"`
}

// handleWSVoice implements the realtime conversation loop:
// audio frames -> VAD -> STT -> wake word -> brain -> TTS -> audio frames.
func (a *API) handleWSVoice(w http.ResponseWriter, r *http.Request) {
	if !a.Voice.Enabled() {
		a.fail(w, r, platform.Unavailable("voice.disabled", "voice is disabled"))
		return
	}
	_, user, err := a.authenticateWS(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	upgrader := a.upgrader()
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	writer := &wsWriter{conn: conn}
	cfg := a.Config.Voice
	sampleRate := cfg.SampleRate
	detector := voice.NewDetector(cfg.VAD, sampleRate)
	profile := voice.DefaultProfile(cfg)
	if a.VoiceProfiles != nil {
		if stored, err := a.VoiceProfiles.Get(r.Context(), user.ID); err == nil && stored != nil {
			profile = stored.Normalize(cfg)
		}
	}

	var (
		mu      sync.Mutex
		cancel  context.CancelFunc
		session string
	)
	barge := func() {
		mu.Lock()
		if cancel != nil {
			cancel()
			cancel = nil
		}
		mu.Unlock()
		detector.Reset()
	}
	if a.Metrics != nil {
		a.Metrics.ActiveVoices.Inc()
		defer a.Metrics.ActiveVoices.Dec()
	}
	_ = writer.json(map[string]any{"type": "ready", "sample_rate": sampleRate, "format": "pcm16"})

	begin := func(ctx context.Context) (context.Context, func()) {
		mu.Lock()
		defer mu.Unlock()
		if cancel != nil {
			cancel()
		}
		turnCtx, turnCancel := context.WithCancel(ctx)
		cancel = turnCancel
		return turnCtx, func() {
			turnCancel()
			mu.Lock()
			cancel = nil
			mu.Unlock()
		}
	}

	respond := func(ctx context.Context, text, agentID, personalityID string) {
		mu.Lock()
		sid := session
		mu.Unlock()
		out, err := a.Brain.Chat(ctx, brain.ChatInput{
			UserID: user.ID, SessionID: sid, Message: text, AgentID: agentID,
			PersonalityID: personalityID, Channel: domain.ChannelVoice,
		}, nil)
		if err != nil {
			_ = writer.json(map[string]any{"type": "error", "stage": "brain", "error": platform.AsError(err).Message})
			return
		}
		mu.Lock()
		session = out.SessionID
		mu.Unlock()
		_ = writer.json(map[string]any{"type": "reply", "text": out.Content, "session_id": out.SessionID,
			"agent": out.AgentID, "provider": out.Provider, "model": out.Model})
		if a.Metrics != nil {
			a.Metrics.ObserveChat(out.AgentID, "voice")
		}
		chunks := voice.SplitSentences(voice.Speakable(out.Content), 220)
		for i, chunk := range chunks {
			select {
			case <-ctx.Done():
				return
			default:
			}
			audio, err := a.Voice.SynthesizeOptions(ctx, chunk, ports.VoiceOptions{
				Voice: profile.Voice, Format: profile.Format, Speed: profile.Speed,
			})
			if err != nil {
				_ = writer.json(map[string]any{"type": "error", "stage": "tts", "error": platform.AsError(err).Message})
				return
			}
			_ = writer.json(map[string]any{"type": "audio", "seq": i, "format": audio.Format, "final": i == len(chunks)-1})
			if err := writer.binary(audio.Data); err != nil {
				return
			}
		}
		_ = writer.json(map[string]any{"type": "audio_end"})
	}

	processAudio := func(pcm []float32) {
		ctx, done := begin(context.Background())
		defer done()
		audio := ports.Audio{
			Data: voice.EncodeWAV(pcm, sampleRate), Format: "wav", SampleRate: sampleRate,
			DurationMS: voice.DurationMS(len(pcm), sampleRate),
		}
		audioWake := a.Voice.AudioWakeEnabled()
		if audioWake {
			activated, wakeErr := a.Voice.DetectWakeWord(ctx, audio)
			if wakeErr != nil {
				_ = writer.json(map[string]any{"type": "error", "stage": "wake", "error": platform.AsError(wakeErr).Message})
				return
			}
			if !activated {
				_ = writer.json(map[string]any{"type": "wake_needed"})
				return
			}
		}
		tr, err := a.Voice.TranscribeLanguage(ctx, audio, profile.Language)
		if err != nil {
			_ = writer.json(map[string]any{"type": "error", "stage": "stt", "error": platform.AsError(err).Message})
			return
		}
		if tr.Text == "" {
			return
		}
		var activated bool
		var command string
		if audioWake {
			activated = true
			command = a.Voice.StripWakeWord(tr.Text)
			if command == "" {
				command = tr.Text
			}
		} else {
			activated, command = a.Voice.WakeWord(tr.Text)
		}
		_ = writer.json(map[string]any{"type": "transcript", "text": tr.Text, "language": tr.Language, "wake": activated})
		if !activated || command == "" {
			_ = writer.json(map[string]any{"type": "wake_needed"})
			return
		}
		respond(ctx, command, "", "")
	}

	processText := func(text, agentID, personalityID string) {
		ctx, done := begin(context.Background())
		defer done()
		respond(ctx, text, agentID, personalityID)
	}

	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			barge()
			return
		}
		switch msgType {
		case websocket.BinaryMessage:
			pcm := voice.BytesToPCM16(data)
			started, ended := detector.Push(pcm, sampleRate)
			if started {
				barge()
				_ = writer.json(map[string]any{"type": "speech_start"})
			}
			if ended {
				utterance := detector.Utterance()
				_ = writer.json(map[string]any{"type": "speech_end", "duration_ms": voice.DurationMS(len(utterance), sampleRate)})
				if len(utterance) > sampleRate/4 {
					go processAudio(utterance)
				}
			}
		case websocket.TextMessage:
			var cmd voiceCommand
			if err := jsonUnmarshal(data, &cmd); err != nil {
				continue
			}
			switch cmd.Type {
			case "interrupt", "cancel":
				barge()
				_ = writer.json(map[string]any{"type": "cancelled"})
			case "text":
				if cmd.SessionID != "" {
					mu.Lock()
					session = cmd.SessionID
					mu.Unlock()
				}
				if cmd.Message != "" {
					go processText(cmd.Message, cmd.AgentID, cmd.PersonalityID)
				}
			case "ping":
				_ = writer.json(map[string]any{"type": "pong", "time": time.Now().UTC()})
			}
		}
	}
}
