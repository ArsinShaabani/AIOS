package voice

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

func TestOpenWakeWordErrorBranches(t *testing.T) {
	status := 200
	body := `{"score":0.99}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	defer srv.Close()
	cfg := config.VoiceConfig{
		Enabled: true, SampleRate: 16000,
		STT: config.STTConfig{Provider: "mock"}, TTS: config.TTSConfig{Provider: "mock"},
		WakeWord: config.WakeWordConfig{Enabled: true, Provider: "openwakeword_http",
			Endpoint: srv.URL, Words: []string{"hey aios"}, Threshold: 0.5},
	}
	svc := NewService(cfg, MockSTT{}, MockTTS{}, nil)
	// A WAV input is forwarded as-is.
	wav := ports.Audio{Data: EncodeWAV(Beep(50, 16000, 400), 16000), Format: "wav"}
	if ok, err := svc.DetectWakeWord(context.Background(), wav); err != nil || !ok {
		t.Fatalf("wav detect: %v %v", ok, err)
	}
	// Threshold 0 defaults to 0.5, so a 0.4 score is rejected.
	body = `{"score":0.4}`
	cfg.WakeWord.Threshold = 0
	svc = NewService(cfg, MockSTT{}, MockTTS{}, nil)
	if ok, err := svc.DetectWakeWord(context.Background(), wav); err != nil || ok {
		t.Fatalf("default threshold: %v %v", ok, err)
	}
	// Non-2xx and malformed JSON are errors, not activations.
	status = http.StatusInternalServerError
	body = `oops`
	if _, err := svc.DetectWakeWord(context.Background(), wav); err == nil {
		t.Fatal("500 accepted")
	}
	status = 200
	body = `not-json`
	if _, err := svc.DetectWakeWord(context.Background(), wav); err == nil {
		t.Fatal("invalid JSON accepted")
	}
	body = `{"score":0.9}`
	if ok, err := svc.DetectWakeWord(context.Background(), wav); err != nil || !ok {
		t.Fatalf("valid after errors: %v %v", ok, err)
	}
}

func TestSpeakAndTranscribeOptions(t *testing.T) {
	cfg := config.VoiceConfig{Enabled: true, SampleRate: 16000,
		STT: config.STTConfig{Provider: "mock", Language: "en"},
		TTS: config.TTSConfig{Provider: "mock", Voice: "default", Format: "wav", Speed: 1}}
	svc := NewService(cfg, MockSTT{}, MockTTS{}, nil)
	audio, err := svc.SynthesizeOptions(context.Background(), "hello", ports.VoiceOptions{Voice: "custom", Format: "wav", Speed: 1.5})
	if err != nil || len(audio.Data) == 0 {
		t.Fatalf("synthesize options: %v", err)
	}
	tr, err := svc.TranscribeLanguage(context.Background(), ports.Audio{Data: []byte{1, 2}, Format: "pcm16", SampleRate: 16000}, "fa")
	if err != nil || tr.Language != "fa" {
		t.Fatalf("transcribe language: %v %+v", err, tr)
	}
	tr, err = svc.TranscribeLanguage(context.Background(), ports.Audio{Data: []byte{1, 2}, Format: "pcm16", SampleRate: 16000}, "")
	if err != nil || tr.Language != "en" {
		t.Fatalf("default language: %v %+v", err, tr)
	}
}

func TestProfileDeleteUnauthorized(t *testing.T) {
	store, _ := profileStore(t)
	if err := store.Delete(context.Background(), ""); err == nil {
		t.Fatal("delete without user accepted")
	}
	if _, err := store.Set(context.Background(), "u1", Profile{Format: "mp3", Speed: 0.5}); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(context.Background(), "u1"); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(context.Background(), "u1"); err != nil {
		t.Fatalf("delete should be idempotent: %v", err)
	}
}
