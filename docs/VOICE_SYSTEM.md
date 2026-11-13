# Voice System

AIOS provides a CPU-friendly voice loop: microphone → VAD → speech-to-text → wake
word → brain → text-to-speech → speaker, with barge-in. One-shot REST endpoints are
available too.

## 1. Pipeline

```
browser mic ──PCM16 16kHz──▶ WS /api/v1/ws/voice
                              ▼
                     adaptive energy VAD
                silence? ──▶ utterance end
                              ▼
                 STT (whisper_http/openai/mock)
                              ▼
                wake word (text match, optional)
                              ▼
                 Brain (same chat pipeline)
                              ▼
              sentence split → TTS per sentence
                              ▼
            JSON events + binary audio ▶ browser
```

## 2. Configuration

```yaml
voice:
  enabled: true
  sample_rate: 16000
  max_session: 30m
  interruptions: true
  stt:
    provider: whisper_http        # or openai / mock
    base_url: http://127.0.0.1:8000/v1
    model: whisper-1
    language: ""                  # auto
  tts:
    provider: piper_http          # or edge_http / openai / mock
    base_url: http://127.0.0.1:5000
    voice: en_US-lessac-medium
    format: wav
    speed: 1.0
  vad:
    energy_threshold: 0.02
    frame_ms: 20
    silence_ms: 700
    min_speech_ms: 200
    max_utterance_ms: 30000
    adaptive: true
  wake_word:
    enabled: false
    words: ["hey aios", "ok aios"]
```

| STT provider | Endpoint | Notes |
|---|---|---|
| `whisper_http` | `POST {base}/audio/transcriptions` | OpenAI-compatible; faster-whisper/whisper.cpp servers |
| `openai` | same | requires `api_key`; base defaults to OpenAI |
| `mock` | — | deterministic “hello aios” for tests |

| TTS provider | Endpoint | Notes |
|---|---|---|
| `piper_http` | `POST {base}/api/tts` | JSON `{text,voice,format,speed}` → audio |
| `edge_http` | `POST {base}/tts` | same contract, Edge-TTS bridge |
| `openai` | `POST {base}/audio/speech` | audio bytes |
| `mock` | — | generated WAV tone, deterministic |

WAV is produced server-side (mono PCM16, resampled to `sample_rate`). Wyoming Piper
is not plain HTTP; run a Piper HTTP wrapper or use `edge_http` in the full compose
profile.

## 3. VAD behaviour

Model-free and allocation-light: 20 ms frames, RMS energy, adaptive noise floor
(slow rise, fast fall), 200 ms minimum speech, end after 700 ms silence, 30 s cap.
A 200 ms pre-roll avoids clipping the first phoneme. `Reset()` implements barge-in.

## 4. WebSocket protocol

`GET /api/v1/ws/voice?token=<jwt>` (browsers cannot set headers).

Client → server:
- binary frames: little-endian PCM16, mono, 16 kHz;
- `{"type":"text","message":"hello","session_id":"ses_…"}` to type;
- `{"type":"interrupt"}` / `{"type":"cancel"}`; `{"type":"ping"}`.

Server → client:
- `{"type":"ready","sample_rate":16000,"format":"pcm16"}`
- `{"type":"speech_start"}` / `{"type":"speech_end","duration_ms":…}`
- `{"type":"transcript","text":…,"language":…,"wake":true}`
- `{"type":"reply","text":…,"session_id":…,"agent":…,"provider":…,"model":…}`
- `{"type":"audio","seq":0,"format":"wav","final":false}` then a binary frame
  (one per sentence), ending with `{"type":"audio_end"}`
- `{"type":"wake_needed"}`, `{"type":"error","stage":"stt|brain|tts","error":…}`

The dashboard (`voice.js`) captures at 16 kHz, streams PCM16, queues audio blobs
and sends `interrupt` on barge-in.

## 5. REST

```bash
curl -s -X POST localhost:8080/api/v1/voice/transcribe \
  -H "Authorization: Bearer $TOKEN" -F audio=@question.wav

curl -s -X POST localhost:8080/api/v1/voice/synthesize \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"text":"Good morning. The kitchen is warm."}' -o reply.wav
```

## 6. Latency and interruptions

One turn per connection; new speech cancels STT/brain/TTS (barge-in). Replies are
split into sentences (≤220 chars) so audio starts before the full answer is ready.
Markdown, code fences and URLs are stripped before synthesis (`Speakable`).

Budgets on a Pi: STT 1.5–4 s for a 5 s utterance with a small model; TTS
150–400 ms per sentence; brain latency depends on the model.

## 7. Privacy

Audio stays in your infrastructure unless you choose a cloud provider. It is
processed in memory and not persisted; transcripts become normal chat messages and
can be deleted from the dashboard.
