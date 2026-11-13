// Package voice implements the voice layer: VAD, WAV handling, STT/TTS engines
// and the conversation-loop service used by the realtime WebSocket transport.
package voice

import (
	"math"

	"github.com/ArsinShaabani/AIOS/internal/config"
)

// Detector is an adaptive energy VAD. It is deliberately model-free so it costs
// almost no CPU and works on a Raspberry Pi; an openWakeWord/Silero adapter can
// replace it behind the same interface.
type Detector struct {
	threshold   float64
	minSpeechMS int
	silenceMS   int
	maxMs       int
	frameMS     int
	adaptive    bool

	noiseFloor float64
	speaking   bool
	speechMs   int
	silenceRun int
	buffer     []float32 // current utterance
	preroll    []float32 // last ~200ms before speech
	prerollMax int
	carry      []float32 // incomplete frame
}

// NewDetector builds a VAD from configuration.
func NewDetector(cfg config.VADConfig, sampleRate int) *Detector {
	if sampleRate <= 0 {
		sampleRate = 16000
	}
	frameMS := cfg.FrameMS
	if frameMS <= 0 {
		frameMS = 20
	}
	silence := cfg.SilenceMS
	if silence < 200 {
		silence = 200
	}
	maxMs := cfg.MaxUtteranceMS
	if maxMs <= 0 {
		maxMs = 30000
	}
	threshold := cfg.EnergyThreshold
	if threshold <= 0 {
		threshold = 0.02
	}
	return &Detector{
		threshold: threshold, minSpeechMS: cfg.MinSpeechMS, silenceMS: silence,
		maxMs: maxMs, frameMS: frameMS, adaptive: cfg.Adaptive,
		noiseFloor: threshold / 2, prerollMax: (200 / frameMS) + 1,
	}
}

// Speaking reports whether an utterance is in progress.
func (d *Detector) Speaking() bool { return d.speaking }

// Push feeds float32 PCM samples and reports whether an utterance started or
// ended on this call. Audio is accumulated internally; call Utterance to take it.
func (d *Detector) Push(samples []float32, sampleRate int) (started, ended bool) {
	if len(samples) == 0 || sampleRate <= 0 {
		return false, false
	}
	frameLen := sampleRate * d.frameMS / 1000
	if frameLen < 1 {
		frameLen = 160
	}
	if len(d.carry) > 0 {
		samples = append(d.carry, samples...)
		d.carry = nil
	}
	for len(samples) >= frameLen {
		frame := samples[:frameLen]
		samples = samples[frameLen:]
		s, e := d.processFrame(frame, sampleRate)
		started = started || s
		ended = ended || e
	}
	if len(samples) > 0 {
		d.carry = append([]float32(nil), samples...)
	}
	return started, ended
}

func (d *Detector) processFrame(frame []float32, sampleRate int) (bool, bool) {
	rms := rmsOf(frame)
	if !d.speaking && d.adaptive {
		// Track the noise floor only while silent; slow rise, fast fall.
		if rms < d.noiseFloor {
			d.noiseFloor = 0.9*d.noiseFloor + 0.1*rms
		} else {
			d.noiseFloor = 0.995*d.noiseFloor + 0.005*rms
		}
	}
	threshold := d.threshold
	if d.adaptive {
		threshold = math.Max(d.threshold, d.noiseFloor*3)
	}
	voice := rms >= threshold

	if !d.speaking {
		d.pushPreroll(frame)
		if !voice {
			return false, false
		}
		d.speechMs += d.frameMS
		if d.speechMs < d.minSpeechMS {
			return false, false
		}
		d.speaking = true
		d.silenceRun = 0
		d.buffer = append(d.buffer, d.preroll...)
		d.buffer = append(d.buffer, frame...)
		d.preroll = nil
		d.speechMs = 0
		return true, false
	}

	d.buffer = append(d.buffer, frame...)
	if voice {
		d.silenceRun = 0
	} else {
		d.silenceRun += d.frameMS
	}
	utteranceMs := len(d.buffer) * 1000 / sampleRate
	if d.silenceRun >= d.silenceMS || utteranceMs >= d.maxMs {
		d.speaking = false
		d.silenceRun = 0
		return false, true
	}
	return false, false
}

func (d *Detector) pushPreroll(frame []float32) {
	d.preroll = append(d.preroll, frame...)
	maxSamples := d.prerollMax * len(frame)
	if len(d.preroll) > maxSamples {
		d.preroll = d.preroll[len(d.preroll)-maxSamples:]
	}
}

// Utterance returns the completed utterance and resets the buffer.
func (d *Detector) Utterance() []float32 {
	out := d.buffer
	d.buffer = nil
	d.preroll = nil
	d.carry = nil
	return out
}

// Reset clears all state (barge-in).
func (d *Detector) Reset() {
	d.speaking = false
	d.speechMs = 0
	d.silenceRun = 0
	d.buffer = nil
	d.preroll = nil
	d.carry = nil
}

// NoiseFloor exposes the current estimate for diagnostics.
func (d *Detector) NoiseFloor() float64 { return d.noiseFloor }

func rmsOf(frame []float32) float64 {
	if len(frame) == 0 {
		return 0
	}
	sum := 0.0
	for _, s := range frame {
		sum += float64(s) * float64(s)
	}
	return math.Sqrt(sum / float64(len(frame)))
}
