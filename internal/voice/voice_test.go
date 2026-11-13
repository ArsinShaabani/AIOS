package voice

import (
	"context"
	"testing"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

func TestVADDetectsSpeech(t *testing.T) {
	cfg := config.VADConfig{EnergyThreshold: 0.02, FrameMS: 20, SilenceMS: 200, MinSpeechMS: 100, MaxUtteranceMS: 5000, Adaptive: false}
	d := NewDetector(cfg, 16000)
	silence := make([]float32, 320)
	started, ended := d.Push(silence, 16000)
	if started || ended {
		t.Fatal("silence must not trigger")
	}
	loud := make([]float32, 320*8)
	for i := range loud {
		loud[i] = 0.3
	}
	started, _ = d.Push(loud, 16000)
	if !started {
		t.Fatal("speech not detected")
	}
	if !d.Speaking() {
		t.Fatal("detector should be speaking")
	}
	ended = false
	for i := 0; i < 20 && !ended; i++ {
		_, ended = d.Push(silence, 16000)
	}
	if !ended {
		t.Fatal("silence did not end the utterance")
	}
	if len(d.Utterance()) == 0 {
		t.Fatal("utterance buffer is empty")
	}
	d.Reset()
	if d.Speaking() {
		t.Fatal("reset failed")
	}
}

func TestWAVRoundTrip(t *testing.T) {
	samples := Beep(120, 16000, 440)
	wav := EncodeWAV(samples, 16000)
	decoded, rate, err := DecodeWAV(wav)
	if err != nil {
		t.Fatal(err)
	}
	if rate != 16000 || len(decoded) != len(samples) {
		t.Fatalf("rate=%d len=%d want %d", rate, len(decoded), len(samples))
	}
	if DurationMS(len(decoded), rate) != 120 {
		t.Fatalf("duration: %d", DurationMS(len(decoded), rate))
	}
	if _, _, err := DecodeWAV([]byte("nope")); err == nil {
		t.Fatal("invalid wav accepted")
	}
	pcm := PCM16ToBytes([]float32{0, 0.5, -0.5, 2, -2})
	back := BytesToPCM16(pcm)
	if len(back) != 5 || back[0] != 0 || back[3] > 1 || back[4] < -1 {
		t.Fatalf("pcm round trip: %+v", back)
	}
	resampled := Resample(samples, 16000, 8000)
	if len(resampled) != len(samples)/2 {
		t.Fatalf("resample length %d", len(resampled))
	}
}

func TestSpeakableAndSplit(t *testing.T) {
	text := "Here is code:\n```go\nfmt.Println(\"hi\")\n```\nSee https://example.com for more. **Done**."
	out := Speakable(text)
	if contains(out, "```") || contains(out, "https://") || contains(out, "**") {
		t.Fatalf("markup survived: %q", out)
	}
	chunks := SplitSentences("First sentence. Second sentence! Third question? "+
		"A very long sentence that keeps going and going and should be split once it passes the limit of characters.", 60)
	if len(chunks) < 3 {
		t.Fatalf("expected multiple chunks, got %v", chunks)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestEnginesAndWakeWord(t *testing.T) {
	cfg := config.VoiceConfig{
		Enabled: true, MaxSession: config.Duration(1e9), SampleRate: 16000,
		STT: config.STTConfig{Provider: "mock"}, TTS: config.TTSConfig{Provider: "mock", Format: "wav"},
		VAD:      config.VADConfig{FrameMS: 20, SilenceMS: 300},
		WakeWord: config.WakeWordConfig{Enabled: true, Words: []string{"hey aios"}},
	}
	stt, tts, err := BuildEngines(cfg)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(cfg, stt, tts, nil)
	if !svc.Enabled() {
		t.Fatal("voice should be enabled")
	}
	woke, cmd := svc.WakeWord("Hey AIOS turn on the light")
	if !woke || cmd != "turn on the light" {
		t.Fatalf("wake word: %v %q", woke, cmd)
	}
	if woke, _ := svc.WakeWord("turn on the light"); woke {
		t.Fatal("wake word should be required")
	}
	tr, err := svc.Transcribe(context.Background(), ports.Audio{Data: []byte{1, 2, 3, 4}, Format: "pcm16", SampleRate: 16000})
	if err != nil || tr.Text == "" {
		t.Fatalf("mock stt: %v %+v", err, tr)
	}
	audio, err := svc.Synthesize(context.Background(), "hello there, this is a test.")
	if err != nil || len(audio.Data) == 0 || audio.Format != "wav" {
		t.Fatalf("mock tts: %v", err)
	}
	if _, err := (MockSTT{}).Transcribe(context.Background(), ports.Audio{}, ""); err == nil {
		t.Fatal("empty audio should fail")
	}
}
