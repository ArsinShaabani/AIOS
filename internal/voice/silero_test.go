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

func TestSileroDetector(t *testing.T) {
	score := 0.9
	var sawAudio, sawRate bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Audio      string `json:"audio"`
			SampleRate int    `json:"sample_rate"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		data, err := base64.StdEncoding.DecodeString(req.Audio)
		sawAudio = err == nil && len(data) > 44
		sawRate = req.SampleRate == 16000
		fmt.Fprintf(w, `{"speech":%f}`, score)
	}))
	defer srv.Close()

	cfg := config.VADConfig{Provider: "silero_http", Endpoint: srv.URL, Threshold: 0.5,
		Timeout: config.Duration(2 * time.Second)}
	det := NewSilero(cfg, 16000)
	audio := ports.Audio{Data: PCM16ToBytes(Beep(120, 16000, 440)), Format: "pcm16", SampleRate: 16000}
	ok, err := det.ConfirmsSpeech(context.Background(), audio)
	if err != nil || !ok || !sawAudio || !sawRate {
		t.Fatalf("confirm: ok=%v err=%v audio=%v rate=%v", ok, err, sawAudio, sawRate)
	}
	score = 0.1
	if ok, _ := det.ConfirmsSpeech(context.Background(), audio); ok {
		t.Fatal("low score confirmed as speech")
	}
	if _, err := det.Speech(context.Background(), ports.Audio{}); err == nil {
		t.Fatal("empty audio accepted")
	}
	cfg.Endpoint = ""
	if _, err := NewSilero(cfg, 16000).Speech(context.Background(), audio); err == nil {
		t.Fatal("missing endpoint accepted")
	}
	cfg.Endpoint = "http://127.0.0.1:1"
	if _, err := NewSilero(cfg, 16000).Speech(context.Background(), audio); err == nil {
		t.Fatal("unreachable bridge accepted")
	}
	// Clamping keeps out-of-range scores sane.
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"speech":7}`)
	}))
	defer srv2.Close()
	cfg.Endpoint = srv2.URL
	if score, err := NewSilero(cfg, 16000).Speech(context.Background(), audio); err != nil || score != 1 {
		t.Fatalf("clamp: %v %v", score, err)
	}
}

func TestServiceConfirmSpeechFallbacks(t *testing.T) {
	base := config.VoiceConfig{Enabled: true, SampleRate: 16000,
		STT: config.STTConfig{Provider: "mock"}, TTS: config.TTSConfig{Provider: "mock"}}
	audio := ports.Audio{Data: PCM16ToBytes(Beep(100, 16000, 440)), Format: "pcm16", SampleRate: 16000}

	// Energy provider: always accepts (no neural check).
	if !NewService(base, MockSTT{}, MockTTS{}, nil).ConfirmSpeech(context.Background(), audio) {
		t.Fatal("energy provider should accept")
	}
	// Broken bridge fails open so no turn is lost.
	bad := base
	bad.VAD = config.VADConfig{Provider: "silero_http", Endpoint: "http://127.0.0.1:1", Threshold: 0.5}
	if !NewService(bad, MockSTT{}, MockTTS{}, nil).ConfirmSpeech(context.Background(), audio) {
		t.Fatal("unreachable silero should fail open")
	}
	// Working bridge rejects silence.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"speech":0.01}`)
	}))
	defer srv.Close()
	good := base
	good.VAD = config.VADConfig{Provider: "silero_http", Endpoint: srv.URL, Threshold: 0.5}
	if NewService(good, MockSTT{}, MockTTS{}, nil).ConfirmSpeech(context.Background(), audio) {
		t.Fatal("silence accepted")
	}
}
