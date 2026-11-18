package voice

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

func TestCloudTTSEngines(t *testing.T) {
	// Coqui server: JSON in, WAV out.
	coqui := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tts" || r.Method != http.MethodPost {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["text"] != "hello" || body["speaker_id"] != "speaker-a" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write([]byte("RIFFwav"))
	}))
	defer coqui.Close()
	ct := NewCoquiTTS(config.TTSConfig{BaseURL: coqui.URL, Voice: "speaker-a", Language: "en",
		Format: "wav", Timeout: config.Duration(2e9)})
	if ct.Name() != "coqui_http" {
		t.Fatal("coqui name")
	}
	audio, err := ct.Synthesize(context.Background(), "hello", ports.VoiceOptions{})
	if err != nil || audio.Format != "wav" || string(audio.Data) != "RIFFwav" {
		t.Fatalf("coqui: %v %+v", err, audio)
	}
	if _, err := ct.Synthesize(context.Background(), " ", ports.VoiceOptions{}); err == nil {
		t.Fatal("empty coqui text accepted")
	}

	// Azure Speech REST.
	var sawKey, sawFormat, sawSSML bool
	azure := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawKey = r.Header.Get("Ocp-Apim-Subscription-Key") == "azure-key"
		sawFormat = strings.Contains(r.Header.Get("X-Microsoft-OutputFormat"), "riff")
		raw, _ := io.ReadAll(r.Body)
		sawSSML = strings.Contains(string(raw), "JennyNeural") && strings.Contains(string(raw), "hello")
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write([]byte("RIFFazure"))
	}))
	defer azure.Close()
	az := NewAzureTTS(config.TTSConfig{BaseURL: azure.URL, APIKey: "azure-key", Voice: "en-US-JennyNeural",
		Format: "wav", Timeout: config.Duration(2e9)})
	if az.Name() != "azure" {
		t.Fatal("azure name")
	}
	audio, err = az.Synthesize(context.Background(), "hello", ports.VoiceOptions{})
	if err != nil || string(audio.Data) != "RIFFazure" || !sawKey || !sawFormat || !sawSSML {
		t.Fatalf("azure: %v %+v key=%v format=%v ssml=%v", err, audio, sawKey, sawFormat, sawSSML)
	}
	if _, err := NewAzureTTS(config.TTSConfig{APIKey: "k"}).Synthesize(context.Background(), "x", ports.VoiceOptions{}); err == nil {
		t.Fatal("azure without endpoint accepted")
	}
	if _, err := NewAzureTTS(config.TTSConfig{BaseURL: azure.URL}).Synthesize(context.Background(), "x", ports.VoiceOptions{}); err == nil {
		t.Fatal("azure without key accepted")
	}
	cfg := config.VoiceConfig{STT: config.STTConfig{Provider: "mock"},
		TTS: config.TTSConfig{Provider: "azure", Region: "westeurope", APIKey: "k"}}
	if _, _, err := BuildEngines(cfg); err != nil {
		t.Fatalf("azure build: %v", err)
	}
	cfg.TTS = config.TTSConfig{Provider: "coqui_http", BaseURL: coqui.URL}
	if _, _, err := BuildEngines(cfg); err != nil {
		t.Fatalf("coqui build: %v", err)
	}
}
