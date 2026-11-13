package voice

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

func TestOpenWakeWordDetection(t *testing.T) {
	score := 0.91
	var gotAudioBytes int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Audio string   `json:"audio"`
			Words []string `json:"words"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		decoded, err := base64.StdEncoding.DecodeString(req.Audio)
		if err != nil || len(req.Words) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		gotAudioBytes = len(decoded)
		fmt.Fprintf(w, `{"score":%f,"word":"hey aios"}`, score)
	}))
	defer srv.Close()

	cfg := config.VoiceConfig{
		Enabled: true, SampleRate: 16000,
		STT: config.STTConfig{Provider: "mock"}, TTS: config.TTSConfig{Provider: "mock"},
		WakeWord: config.WakeWordConfig{Enabled: true, Provider: "openwakeword_http",
			Endpoint: srv.URL, Words: []string{"hey aios"}, Threshold: 0.5, Timeout: config.Duration(2 * time.Second)},
	}
	svc := NewService(cfg, MockSTT{}, MockTTS{}, nil)
	if !svc.AudioWakeEnabled() {
		t.Fatal("audio wake should be enabled")
	}
	audio := ports.Audio{Data: PCM16ToBytes(Beep(100, 16000, 440)), Format: "pcm16", SampleRate: 16000}
	ok, err := svc.DetectWakeWord(context.Background(), audio)
	if err != nil || !ok {
		t.Fatalf("detect: %v %v", ok, err)
	}
	if gotAudioBytes < 44 {
		t.Fatalf("server received %d bytes", gotAudioBytes)
	}
	score = 0.1
	if ok, _ := svc.DetectWakeWord(context.Background(), audio); ok {
		t.Fatal("low score activated")
	}
	if _, err := svc.DetectWakeWord(context.Background(), ports.Audio{}); err == nil {
		t.Fatal("empty audio accepted")
	}
	score = 0.9
	cfg.WakeWord.Endpoint = "http://127.0.0.1:1"
	svc = NewService(cfg, MockSTT{}, MockTTS{}, nil)
	if _, err := svc.DetectWakeWord(context.Background(), audio); err == nil {
		t.Fatal("unreachable wake service accepted")
	}
	cfg.WakeWord.Endpoint = ""
	svc = NewService(cfg, MockSTT{}, MockTTS{}, nil)
	if _, err := svc.DetectWakeWord(context.Background(), audio); err == nil {
		t.Fatal("missing endpoint accepted")
	}
	// Text provider never performs audio detection.
	textCfg := cfg
	textCfg.WakeWord.Provider = "text"
	textCfg.WakeWord.Enabled = true
	textSvc := NewService(textCfg, MockSTT{}, MockTTS{}, nil)
	if textSvc.AudioWakeEnabled() {
		t.Fatal("text provider reported audio wake")
	}
	if ok, err := textSvc.DetectWakeWord(context.Background(), audio); err != nil || ok {
		t.Fatalf("text audio detection: %v %v", ok, err)
	}
}

func TestStripWakeWord(t *testing.T) {
	cfg := config.VoiceConfig{WakeWord: config.WakeWordConfig{Enabled: true,
		Words: []string{"hey aios", "ok aios"}}}
	svc := NewService(cfg, MockSTT{}, MockTTS{}, nil)
	if got := svc.StripWakeWord("Hey AIOS, turn on the light"); got != "turn on the light" {
		t.Fatalf("strip: %q", got)
	}
	if got := svc.StripWakeWord("turn on the light, ok aios"); got != "turn on the light," {
		t.Fatalf("strip mid-sentence: %q", got)
	}
	if got := svc.StripWakeWord("no wake word"); got != "no wake word" {
		t.Fatalf("strip unchanged: %q", got)
	}
}
