# AIOS

**Personal AI Operating System** — a local-first AI core that runs on a 1 CPU /
1 GB Linux box, a Raspberry Pi or a mini PC. Chat, agents, long-term memory,
personality, voice, devices, plugins and an admin dashboard in a single Go binary.

```
┌────────────────────────────────────────────────────────────┐
│ Chat · Voice · Devices · Agents · Memory · Plugins · Admin │
├────────────────────────────────────────────────────────────┤
│ Brain: intent → agent → model router → tools → streaming   │
│ Memory: short-term · long-term · semantic (CPU embeddings) │
│ Providers: local · Ollama · OpenAI · Anthropic · Gemini …  │
│ Edge: SQLite · in-memory cache/queue · no GPU · no cgo     │
└────────────────────────────────────────────────────────────┘
```

## Why it is different

- **Actually small.** Pure-Go SQLite, bounded caches/queues, GOGC/GOMEMLIMIT
  defaults; idle RSS stays far below the 1 GB budget. Cross-compiles to ARM.
- **Not locked to a model.** Add Ollama, OpenAI, Anthropic, Gemini, DeepSeek,
  Mistral or any OpenAI-compatible server. Fallbacks, circuit breakers, cost
  accounting and per-intent routing live in the router.
- **Agentic but safe.** Tool calls are typed, time-boxed and capability-bounded;
  device commands cannot exceed declared min/max limits; web fetches block private
  IPs; secrets never reach logs.
- **Memory that works offline.** Lexical hash embeddings by default, real
  embedding providers when available. Farsi and English extraction built in.
- **Runs alone.** Dashboard, OpenAPI spec and GraphQL playground are embedded in
  the binary — no npm build step, no CDN, no external service required.

## Install on a server (one command)

```bash
curl -fsSL https://github.com/ArsinShaabani/AIOS/releases/latest/download/install.sh \
  | sudo bash -s -- --admin-password 'Choose-A-Strong-Password'
```

- amd64 / arm64 / armv7 (Raspberry Pi) assets are detected automatically.
- Upgrade in place (keeps config and data): `--upgrade`.
- Offline install: `sudo bash install.sh --local-binary ./aios_1.2.1_linux_amd64`.
- Pin a version: `--version v1.2.1`.
- Without systemd (containers): `--no-systemd`.

The installer verifies the SHA-256 checksum, creates a hardened systemd unit and
prints the generated admin password (also stored in
`/etc/aios/admin-password.txt`).

Installs a static binary, hardened systemd service, production config and the
admin dashboard on `http://<server>:8080`. See [docs/INSTALL.md](docs/INSTALL.md).

## Quick start from source

```bash
git clone https://github.com/ArsinShaabani/AIOS.git && cd AIOS
go build -trimpath -o aios ./cmd/aios
cp config.example.yaml config.yaml        # edit as needed
AIOS_SECURITY_BOOTSTRAP_PASSWORD='change-me-now' ./aios server
```

Open `http://127.0.0.1:8080` and sign in as `admin@aios.local`.

For real answers, run a local model:

```bash
ollama pull qwen2.5:1.5b
AIOS_LLM_DEFAULT_PROVIDER=ollama AIOS_LLM_DEFAULT_MODEL=qwen2.5:1.5b ./aios server
```

Docker: `AIOS_ADMIN_PASSWORD=... docker compose up -d --build`.
Full stack (Postgres/Redis/Qdrant/Ollama/Whisper/Piper):
`docker compose -f deploy/docker-compose.full.yml up -d`.

## What is included

| Area | Status |
|---|---|
| REST + SSE + WebSocket + GraphQL APIs | ✅ |
| Streaming chat with tool-calling loop | ✅ |
| Providers: local, Ollama, OpenAI-compatible, Anthropic, Gemini | ✅ |
| Routing rules, fallbacks, breakers, token/cost accounting | ✅ |
| Three-tier memory + extraction (EN/FA) + semantic recall | ✅ |
| Document RAG: text/HTML/PDF upload, chunk+embed, citations, document_search tool | ✅ |
| Personalities, agents, prompt viewer | ✅ |
| Devices: MQTT, HTTP, capability validation, telemetry | ✅ |
| Authenticated MQTT chat bridge (API keys, rate limits) | ✅ |
| Home Assistant, navigation, email, reminders/automations | ✅ |
| Voice: VAD, Whisper, Piper/Edge/OpenAI TTS, wake word, barge-in | ✅ |
| Plugins: HTTP tools + WASM sandbox (wazero) + permissions | ✅ |
| Plugin supply chain: Ed25519 signatures, trusted keys, marketplace manifest | ✅ |
| Security: JWT + refresh rotation, RBAC, audit, AES-GCM, SSRF guard | ✅ |
| Admin SPA, Prometheus metrics, health, audit | ✅ |
| Docker (multi-arch), compose profiles, systemd, CI/release | ✅ |
| Postgres / Redis / Qdrant adapters | ✅ (opt-in) |
| Queue visibility timeout + stale task reclaim | ✅ |
| PostgreSQL integration tests in CI | ✅ |
| Event bus: in-memory or NATS (horizontal scaling) | ✅ |
| Cross-replica chat fan-out + MQTT shared subscriptions | ✅ |
| Test coverage | ✅ 80%+ enforced in CI (currently ~80.8%; see [DESIGN.md](docs/DESIGN.md#7-test-strategy)) |

Roadmap, MVP definition and risks: [docs/DESIGN.md](docs/DESIGN.md).

## Documentation

- [Architecture](docs/ARCHITECTURE.md) · [Design details + ERD + roadmap](docs/DESIGN.md)
- [API reference](docs/API.md) · OpenAPI at `/openapi.yaml`
- [Deployment](docs/DEPLOYMENT.md) · [Security](docs/SECURITY.md)
- [Voice system](docs/VOICE_SYSTEM.md) · [Device integration](docs/DEVICE_INTEGRATION.md)
- [Plugin development](docs/PLUGIN_DEVELOPMENT.md) · [User guide](docs/USER_GUIDE.md)

## Development

```bash
make test        # unit + e2e
make race        # race detector
make cover       # coverage profile
make lint        # gofmt + vet
make build       # bin/aios
make release     # linux amd64/arm64/armv7 + checksums
```

Layout: `internal/{domain,ports,config,security,memory,brain,llm,tools,devices,…}`,
adapters under `internal/adapters`, transports under `internal/transport`, single
composition root in `internal/app`.

## License

MIT — see [LICENSE](LICENSE).

## Web GUI (v1.1)

- **Chat** with streaming Markdown, syntax-styled code blocks + copy, animated
  *Thinking…* state with elapsed timer, inline tool-run cards, stop button, token
  usage and character/model pickers.
- **Models**: add any OpenAI-compatible, Anthropic, Gemini or Ollama provider with
  an API key (encrypted at rest), test it with a real completion (tok/s + latency),
  switch the global default, and pull/delete Ollama models with progress. One-click
  presets for keyless local servers and free tiers — see [docs/MODELS.md](docs/MODELS.md).
- **Characters**: gallery + editor for tone, humor, warmth, length and custom
  instructions; per-browser default character.
- **Settings**: dark/light/system theme, accent color, font size, density, motion,
  account and data export. All personalization is stored locally.

## What's new in v1.2

- **Per-provider/model limits**: `max_tokens`, `context_window`, `temperature`,
  `top_p`, `reasoning_effort` — fixes truncated answers on large models. Chat only
  exposes reasoning effort; token limits and sampling come from the provider.
  Message ceiling raised to 200k characters.
- **Model discovery**: “Fetch models from provider” after adding an API key
  (OpenAI-compatible, Ollama, Anthropic, Gemini). Free/no-key presets included:
  Ollama, LM Studio, llama.cpp, vLLM, Pollinations; free tiers: Groq, Gemini,
  OpenRouter `:free`, Mistral, Cerebras, Hugging Face, DeepSeek.
- **Skills & Agents**: 12 installable skill templates with prompts, tool
  allow-lists and routing keywords; full editor in the UI.
- **Chat**: file/image attachments, sandboxed **code → preview** (HTML/CSS/JS/SVG/
  Markdown), character gallery picker, advanced options, stop/regenerate.
- **Appearance**: code font size, chat width + existing theme/accent/density.

See [docs/V1.2.md](docs/V1.2.md).

## Bring your own AI (v1.3)

Every user can connect their **own** provider keys and use AIOS as an
OpenAI-compatible tunnel for any external app or agent:

1. **هوش مصنوعی من** → add Ollama (keyless), Groq, Gemini, OpenRouter, OpenAI,
   Anthropic or any OpenAI-compatible endpoint. Keys are encrypted with AES-GCM
   and never returned by the API.
2. Create a tunnel API key bound to one provider + model.
3. Point any OpenAI SDK/agent at `https://your-host/v1` with that key. The real
   upstream key never leaves the server; the model can be fixed per key.

```bash
curl https://your-host/v1/chat/completions \
  -H "Authorization: Bearer aios_..." \
  -H "Content-Type: application/json" \
  -d '{"model":"auto","messages":[{"role":"user","content":"سلام"}]}'
```

Also in v1.3: self-registration, richer device profiles (connection, tone,
language, requirements, preferred model), full server backup/restore in
Settings, RTL dashboard with Persian onboarding, 4k default output / 8k context,
and a 2 GB / 2 vCPU systemd profile (`GOMEMLIMIT=768MiB`, `GOGC=50`).
