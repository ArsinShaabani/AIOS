package voice

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

func TestOpenAITTS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer k" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("mp3data"))
	}))
	defer srv.Close()
	tts := NewOpenAITTS(config.TTSConfig{BaseURL: srv.URL, APIKey: "k", Model: "m", Voice: "alloy",
		Format: "mp3", Timeout: config.Duration(2e9)})
	audio, err := tts.Synthesize(context.Background(), "hello", ports.VoiceOptions{})
	if err != nil || audio.Format != "mp3" || string(audio.Data) != "mp3data" {
		t.Fatalf("openai tts: %v %+v", err, audio)
	}
	bad := NewOpenAITTS(config.TTSConfig{BaseURL: srv.URL, APIKey: "wrong", Timeout: config.Duration(2e9)})
	if _, err := bad.Synthesize(context.Background(), "hello", ports.VoiceOptions{}); err == nil {
		t.Fatal("unauthorized tts accepted")
	}
}

func TestBuildEnginesValidation(t *testing.T) {
	base := config.VoiceConfig{
		STT: config.STTConfig{Provider: "mock"},
		TTS: config.TTSConfig{Provider: "mock"},
	}
	if _, _, err := BuildEngines(base); err != nil {
		t.Fatalf("mock engines: %v", err)
	}
	cases := []config.VoiceConfig{
		{STT: config.STTConfig{Provider: "bogus"}, TTS: config.TTSConfig{Provider: "mock"}},
		{STT: config.STTConfig{Provider: "openai"}, TTS: config.TTSConfig{Provider: "mock"}},
		{STT: config.STTConfig{Provider: "mock"}, TTS: config.TTSConfig{Provider: "piper_http"}},
		{STT: config.STTConfig{Provider: "mock"}, TTS: config.TTSConfig{Provider: "openai"}},
		{STT: config.STTConfig{Provider: "mock"}, TTS: config.TTSConfig{Provider: "bogus"}},
	}
	for _, c := range cases {
		if _, _, err := BuildEngines(c); err == nil {
			t.Fatalf("expected validation failure for %+v", c)
		}
	}
	cfg := config.VoiceConfig{
		Enabled: true, SampleRate: 16000,
		STT: config.STTConfig{Provider: "mock"}, TTS: config.TTSConfig{Provider: "mock", Format: "wav"},
		VAD: config.VADConfig{FrameMS: 20, SilenceMS: 300},
	}
	stt, tts, _ := BuildEngines(cfg)
	svc := NewService(cfg, stt, tts, nil)
	if _, err := svc.Transcribe(context.Background(), ports.Audio{}); err == nil {
		t.Fatal("empty transcription accepted")
	}
	disabled := NewService(config.VoiceConfig{}, nil, nil, nil)
	if disabled.Enabled() {
		t.Fatal("disabled service reports enabled")
	}
	if _, err := disabled.Synthesize(context.Background(), "x"); err == nil {
		t.Fatal("disabled synthesize accepted")
	}
}

func TestVADEdgeCases(t *testing.T) {
	cfg := config.VADConfig{EnergyThreshold: 0.02, FrameMS: 20, SilenceMS: 200, MinSpeechMS: 60, MaxUtteranceMS: 120, Adaptive: true}
	d := NewDetector(cfg, 16000)
	loud := make([]float32, 320*20)
	for i := range loud {
		loud[i] = 0.5
	}
	started, ended := d.Push(loud, 16000)
	if !started {
		t.Fatal("speech not detected")
	}
	if !ended {
		t.Fatal("max utterance cap not enforced")
	}
	if len(d.Utterance()) == 0 {
		t.Fatal("utterance empty")
	}
	if d.NoiseFloor() <= 0 {
		t.Fatal("noise floor not tracked")
	}
	if s, e := d.Push(nil, 16000); s || e {
		t.Fatal("empty push produced events")
	}
}

func TestSpeakableVariants(t *testing.T) {
	out := Speakable("## Title\n\nUse `code` and see https://example.com/x for **details**.")
	if strings.Contains(out, "#") || strings.Contains(out, "http") || strings.Contains(out, "**") {
		t.Fatalf("markup survived: %q", out)
	}
	long := strings.Repeat("word ", 400)
	if len([]rune(Speakable(long))) > 1200 {
		t.Fatal("speakable text not truncated")
	}
	chunks := SplitSentences("One. Two! Three? "+strings.Repeat("x", 300), 100)
	if len(chunks) < 3 {
		t.Fatalf("chunks: %v", chunks)
	}
}
