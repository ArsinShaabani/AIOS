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
curl -fsSL https://raw.githubusercontent.com/ArsinShaabani/AIOS/main/deploy/install.sh \
  | sudo bash -s -- --admin-password 'Choose-A-Strong-Password'
```

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
