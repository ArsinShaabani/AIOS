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

func TestWhisperSTT(t *testing.T) {
	wav := EncodeWAV(Beep(100, 16000, 440), 16000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if err := r.ParseMultipartForm(4 << 20); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if _, _, err := r.FormFile("file"); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, `{"text":" hello world ","language":"en","duration":1.25}`)
	}))
	defer srv.Close()
	cfg := config.STTConfig{Provider: "whisper_http", BaseURL: srv.URL, APIKey: "key", Model: "whisper-1", Timeout: config.Duration(3e9)}
	stt := NewWhisperSTT(cfg)
	if stt.Name() != "whisper_http" {
		t.Fatal("name")
	}
	tr, err := stt.Transcribe(context.Background(), ports.Audio{Data: wav, Format: "wav"}, "fa")
	if err != nil || tr.Text != "hello world" || tr.DurationMS != 1250 {
		t.Fatalf("transcribe: %v %+v", err, tr)
	}
	if _, err := stt.Transcribe(context.Background(), ports.Audio{}, ""); err == nil {
		t.Fatal("empty audio accepted")
	}
	pcm := PCM16ToBytes(Beep(80, 16000, 440))
	if _, err := stt.Transcribe(context.Background(), ports.Audio{Data: pcm, Format: "pcm16", SampleRate: 16000}, ""); err != nil {
		t.Fatalf("pcm transcribe: %v", err)
	}
	bad := NewWhisperSTT(config.STTConfig{BaseURL: srv.URL, APIKey: "wrong", Timeout: config.Duration(2e9)})
	if _, err := bad.Transcribe(context.Background(), ports.Audio{Data: wav, Format: "wav"}, ""); err == nil {
		t.Fatal("unauthorized STT accepted")
	}
	missing := NewWhisperSTT(config.STTConfig{BaseURL: "http://127.0.0.1:1", Timeout: config.Duration(1e9)})
	if _, err := missing.Transcribe(context.Background(), ports.Audio{Data: wav, Format: "wav"}, ""); err == nil {
		t.Fatal("unreachable STT accepted")
	}
}

func TestHTTPTTS(t *testing.T) {
	wav := EncodeWAV(Beep(100, 16000, 440), 16000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tts", "/tts", "/", "/missing/":
			w.Header().Set("Content-Type", "audio/wav")
			_, _ = w.Write(wav)
		case "/fail/api/tts":
			w.WriteHeader(http.StatusInternalServerError)
		case "/missing/api/tts":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	piper := NewPiperTTS(config.TTSConfig{BaseURL: srv.URL, Voice: "v", Format: "wav", Timeout: config.Duration(3e9)})
	if piper.Name() != "piper_http" {
		t.Fatal("name")
	}
	audio, err := piper.Synthesize(context.Background(), "hello", ports.VoiceOptions{})
	if err != nil || len(audio.Data) == 0 || audio.Format != "wav" {
		t.Fatalf("piper: %v %+v", err, audio)
	}
	if _, err := piper.Synthesize(context.Background(), "  ", ports.VoiceOptions{}); err == nil {
		t.Fatal("empty text accepted")
	}
	failing := NewPiperTTS(config.TTSConfig{BaseURL: srv.URL + "/fail", Timeout: config.Duration(2e9)})
	if _, err := failing.Synthesize(context.Background(), "hello", ports.VoiceOptions{}); err == nil {
		t.Fatal("500 accepted")
	}
	fallback := NewPiperTTS(config.TTSConfig{BaseURL: srv.URL + "/missing", Timeout: config.Duration(2e9)})
	if _, err := fallback.Synthesize(context.Background(), "hello", ports.VoiceOptions{}); err != nil {
		t.Fatalf("404 fallback failed: %v", err)
	}
	edge := NewEdgeTTS(config.TTSConfig{BaseURL: srv.URL, Timeout: config.Duration(2e9)})
	if edge.Name() != "edge_http" {
		t.Fatal("edge name")
	}
	if _, err := edge.Synthesize(context.Background(), "hi", ports.VoiceOptions{}); err != nil {
		t.Fatal(err)
	}
	unreachable := NewPiperTTS(config.TTSConfig{BaseURL: "http://127.0.0.1:1", Timeout: config.Duration(1e9)})
	if _, err := unreachable.Synthesize(context.Background(), "hi", ports.VoiceOptions{}); err == nil {
		t.Fatal("unreachable TTS accepted")
	}
}
