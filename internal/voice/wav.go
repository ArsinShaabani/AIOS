package voice

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
)

// BytesToPCM16 converts little-endian PCM16 bytes to float32 samples in [-1,1].
func BytesToPCM16(data []byte) []float32 {
	n := len(data) / 2
	out := make([]float32, n)
	for i := 0; i < n; i++ {
		v := int16(binary.LittleEndian.Uint16(data[i*2:]))
		out[i] = float32(v) / 32768
	}
	return out
}

// PCM16ToBytes converts float32 samples to little-endian PCM16 bytes.
func PCM16ToBytes(samples []float32) []byte {
	out := make([]byte, len(samples)*2)
	for i, s := range samples {
		if s > 1 {
			s = 1
		} else if s < -1 {
			s = -1
		}
		binary.LittleEndian.PutUint16(out[i*2:], uint16(int16(s*32767)))
	}
	return out
}

// WAVHeaderSize is the canonical 44-byte PCM WAV header.
const WAVHeaderSize = 44

// EncodeWAV wraps PCM16 samples in a mono 16-bit WAV container.
func EncodeWAV(samples []float32, sampleRate int) []byte {
	if sampleRate <= 0 {
		sampleRate = 16000
	}
	pcm := PCM16ToBytes(samples)
	var buf bytes.Buffer
	buf.Grow(WAVHeaderSize + len(pcm))
	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, uint32(36+len(pcm)))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	binary.Write(&buf, binary.LittleEndian, uint32(16))
	binary.Write(&buf, binary.LittleEndian, uint16(1)) // PCM
	binary.Write(&buf, binary.LittleEndian, uint16(1)) // mono
	binary.Write(&buf, binary.LittleEndian, uint32(sampleRate))
	binary.Write(&buf, binary.LittleEndian, uint32(sampleRate*2))
	binary.Write(&buf, binary.LittleEndian, uint16(2))
	binary.Write(&buf, binary.LittleEndian, uint16(16))
	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, uint32(len(pcm)))
	buf.Write(pcm)
	return buf.Bytes()
}

// DecodeWAV parses PCM16 WAV files and returns mono float32 samples.
func DecodeWAV(data []byte) ([]float32, int, error) {
	if len(data) < WAVHeaderSize || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, 0, errors.New("not a WAV file")
	}
	var (
		sampleRate int
		channels   int
		bits       int
		audio      []byte
	)
	offset := 12
	for offset+8 <= len(data) {
		id := string(data[offset : offset+4])
		size := int(binary.LittleEndian.Uint32(data[offset+4:]))
		body := offset + 8
		if body+size > len(data) {
			size = len(data) - body
		}
		switch id {
		case "fmt ":
			if size >= 16 {
				channels = int(binary.LittleEndian.Uint16(data[body+2:]))
				sampleRate = int(binary.LittleEndian.Uint32(data[body+4:]))
				bits = int(binary.LittleEndian.Uint16(data[body+14:]))
			}
		case "data":
			audio = data[body : body+size]
		}
		offset = body + size
		if size%2 == 1 {
			offset++ // chunks are word-aligned
		}
	}
	if sampleRate == 0 || channels == 0 {
		return nil, 0, errors.New("WAV is missing fmt/data chunks")
	}
	if bits != 16 {
		return nil, 0, errors.New("only 16-bit PCM WAV is supported")
	}
	if channels < 1 || channels > 2 {
		return nil, 0, errors.New("only mono or stereo WAV is supported")
	}
	mono := make([]float32, 0, len(audio)/2/channels)
	for i := 0; i+2*channels <= len(audio); i += 2 * channels {
		sum := 0.0
		for c := 0; c < channels; c++ {
			sum += float64(int16(binary.LittleEndian.Uint16(audio[i+c*2:]))) / 32768
		}
		mono = append(mono, float32(sum/float64(channels)))
	}
	return mono, sampleRate, nil
}

// Resample linearly converts samples between sample rates.
func Resample(samples []float32, from, to int) []float32 {
	if from <= 0 || to <= 0 || from == to || len(samples) == 0 {
		return samples
	}
	ratio := float64(to) / float64(from)
	outLen := int(float64(len(samples)) * ratio)
	out := make([]float32, outLen)
	for i := 0; i < outLen; i++ {
		pos := float64(i) / ratio
		idx := int(pos)
		frac := float32(pos - float64(idx))
		a := samples[idx]
		var b float32
		if idx+1 < len(samples) {
			b = samples[idx+1]
		}
		out[i] = a + (b-a)*frac
	}
	return out
}

// DurationMS returns the duration of n samples at a rate.
func DurationMS(n, sampleRate int) int64 {
	if sampleRate <= 0 {
		return 0
	}
	return int64(float64(n) / float64(sampleRate) * 1000)
}

// Beep synthesises a short sine tone with fades (mock TTS).
func Beep(durationMS int, sampleRate int, freq float64) []float32 {
	if durationMS <= 0 {
		durationMS = 300
	}
	if sampleRate <= 0 {
		sampleRate = 16000
	}
	n := sampleRate * durationMS / 1000
	out := make([]float32, n)
	fade := minInt(n/10, sampleRate/50)
	for i := 0; i < n; i++ {
		v := math.Sin(2 * math.Pi * freq * float64(i) / float64(sampleRate))
		amp := 0.25
		if i < fade {
			amp *= float64(i) / float64(fade)
		}
		if i > n-fade && fade > 0 {
			amp *= float64(n-i) / float64(fade)
		}
		out[i] = float32(v * amp)
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
