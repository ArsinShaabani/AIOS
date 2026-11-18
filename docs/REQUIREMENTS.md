# AIOS requirements audit

This document maps the original master prompt (and the v1.3 follow-up requests)
to what is actually implemented, with evidence. Status values:

- **Done** — implemented, wired, tested and documented.
- **Partial** — the core works but a named sub-item is not native (roadmap).
- **External** — code exists but activation depends on the deployment/token.

Last audited: v1.3.0 + the follow-up fixes (Coqui/Azure TTS, `/docs`, character and
security navigation).

## Product and core

| Requirement | Status | Evidence / notes |
|---|---|---|
| Lightweight AI core on weak Linux, no GPU, low RAM/CPU | Done | Go (no cgo), lazy engines, bounded pools; `deploy/systemd`, Docker; 2 CPU/2 GB profile in `docs/DEPLOYMENT.md` |
| Raspberry Pi / Mini PC | Done | `make release` cross-compiles linux/arm64 + armv7; pure-Go SQLite |
| Thousands of devices | Partial | Connection registries, MQTT/WebSocket/HTTP transports, per-device tokens and cache; a single node is bounded by network/fd limits. Horizontal scaling uses Redis + NATS + Postgres |
| Robots, IoT, mobile/desktop apps | Done | Device API + WebSocket voice/chat + REST/GraphQL + OpenAI-compatible `/v1` tunnel |
| Public API | Done | REST, GraphQL, WebSocket, MQTT bridge, OpenAPI 3.0.3, `/docs` |
| Personalities | Done | Personality engine + presets + per-user characters, chat picker, `views_character.js` in the sidebar |
| Long-term / semantic memory | Done | 3 tiers, sqlite memories, local vector store or Qdrant, consolidation + summarizer |
| Learning / optimization | Partial | Memory extraction, usage-aware routing, caching and consolidation; no online weight training (by design for CPU-only edge) |

## Services and architecture

| Requirement | Status | Evidence / notes |
|---|---|---|
| Gateway (API/WS/MQTT/voice/devices) | Done | `internal/transport/httpapi`, `internal/adapters/mqtt`, `ws.go` |
| Brain engine | Done | Intent → agent selection → context builder → tool loop → streaming (`internal/brain`) |
| Agent manager + 10 agents | Done | Search, Home Assistant, Robotics, IoT, Scheduler, Email, Device, Navigation, Automation, Knowledge (+ skill catalog) |
| Memory system | Done | Short/long/semantic, retention, wipe, admin viewer endpoint |
| Personality engine | Done | Tone/style/humor/warmth/length compiled into system prompt |
| Plugin system | Done | HTTP plugins + signed WASM sandbox (wazero, fresh runtime per call), manifest loader, permissions, enable/disable |
| Microservice orientation | Partial (by design) | Modular monolith with hexagonal ports, extractable modules, `compose.micro.yaml`; a 12-process split is not viable on 1 GB |
| Clean Architecture / SOLID / DDD / CQRS-ready / DI / Hexagonal | Done | `domain`, `ports`, `app`, adapters; documented in `docs/ARCHITECTURE.md` |

## Performance

| Requirement | Status | Evidence |
|---|---|---|
| Lazy loading, async, worker queues, cache, streaming | Done | Lazy voice/tools, bounded queue workers, Redis/in-memory cache, SSE/WS streaming |
| Redis + message queue | Done | Redis cache/queue + NATS event bus; in-memory fallbacks |
| Visibility timeout / crash recovery | Done | Redis in-flight ZSET reclaim + durable task reclaim (`v1.3.0`) |

## LLM layer

| Requirement | Status | Evidence |
|---|---|---|
| Provider architecture, no core dependency | Done | `ports.Provider`, router, fallbacks, breaker |
| OpenAI, Anthropic, Gemini, DeepSeek, Mistral, Ollama, GGUF, custom | Done | DeepSeek/Mistral/llama.cpp use the OpenAI-compatible adapter; GGUF via Ollama/llama.cpp; user BYOK providers |
| Per-user provider + key + model | Done | `user_providers`, AES-GCM at rest, `/api/v1/me/providers`, `/api/v1/me/models` |
| API keys that external apps/agents can use | Done | Bound `aios_` keys → `/v1/chat/completions`, `/v1/models` with streaming/tools/usage |
| Long answers | Done | 4096 output / 8192 context defaults; per-provider `max_tokens`/`context_window` |


## Voice

| Requirement | Status | Evidence |
|---|---|---|
| STT: Whisper, faster-whisper, custom | Done | OpenAI-compatible Whisper HTTP; faster-whisper/whisper.cpp servers use the same contract |
| TTS: Piper, Coqui, Edge, Azure | Done | `piper_http`, `coqui_http`, `edge_http`, `azure` (+ OpenAI, mock), tested with HTTP servers |
| Realtime loop, VAD, interrupt/barge-in, wake word | Done | WS voice loop, energy/Silero VAD, wake-word gate, cancel on interrupt |
| OPUS transport | Partial | Format is negotiated and the engine can emit Opus, but the browser loop ships PCM/WAV; no native Opus codec in the core |

## Robotics and smart home

| Requirement | Status | Evidence |
|---|---|---|
| ESP32, Raspberry Pi, STM32, Arduino, ROS2 | Done (protocols) | MQTT/HTTP/WS/TCP/rosbridge transports, capability validation, safety bounds, docs. No native ROS2 DDS; use rosbridge |
| Home Assistant | Done | REST adapter + tools/agent |
| Zigbee gateways, Shelly, Sonoff | Done (via MQTT) | MQTT topics for Zigbee2MQTT/Tasmota/Shelly/Sonoff |
| Matter / Tuya | **Not implemented natively** | Generic MQTT/HTTP devices only. Roadmap |
| Internet search/read/summarize/multi-step research/compare/fact-check | Done | Search agent + deep-researcher skill, fetch/extract, knowledge, citations |

## Security

| Requirement | Status | Evidence |
|---|---|---|
| JWT + refresh, RBAC, audit, encryption at rest/in transit, secrets | Done | `internal/security`, TLS listener or reverse proxy, AES-256-GCM keyring, audit log, bcrypt |
| Registration / user management | Done | `/auth/register` (configurable), admin CRUD, role/status/reset |
| API key scoping/rotation | Done | Hashed keys, expiry, scopes, provider/model binding, revoke |
| Email verification / password reset by email | **Not implemented** | Requires an SMTP policy/flow; admin reset exists |
| Self-service password change | **Not implemented** | Admin reset endpoint only |

## Data, API and admin

| Requirement | Status | Evidence |
|---|---|---|
| SQLite default, PostgreSQL, **MariaDB** | Partial | SQLite + PostgreSQL are tested. MariaDB/MySQL is not supported: `database.driver` rejects `mysql`; upserts use `ON CONFLICT` |
| Repository pattern, no core changes on DB swap | Done | Ports + sqlstore; MariaDB would need a dialect adapter |
| REST/GraphQL/WS/MQTT/OpenAPI/Swagger | Done | `/docs` Redoc shell + raw `/openapi.yaml` |
| Admin dashboard: overview/users/devices/agents/plugins/logs/analytics/AI usage/costs/settings/security/memory/prompt/router/queue | Done | Views in `static/`; Security page added; Prompt viewer in chat; Model Router and Task Queue pages |
| Backup/restore everything | Partial | JSON backup of users/settings/keys/providers/devices/personalities/agents/plugins + optional conversations/memories/documents. Restore requires the same keyring for encrypted provider keys |
| Advanced settings UI | Partial | Dashboard covers appearance, account, data/backup, providers, models, voice profile and characters. AI routing, voice engine choice and memory retention are still edited in `config.yaml`/env (validated, hot-reload not provided) |
| Monitoring (Prometheus/Grafana) | Done | `/metrics`, `deploy/prometheus`, `deploy/grafana` |

## Engineering rules

| Requirement | Status | Evidence |
|---|---|---|
| Go backend | Done | Go 1.27, single binary |
| Main branch, Conventional Commits, SemVer | Done | `main`, conventional history, `v1.3.0` tag |
| Unit/integration/E2E/load tests | Done | 209 test functions, E2E suite, `tests/load` (build tag) |
| ≥80% coverage | Done | 80%+ merged gate (`scripts/coverage.sh`, `coverage-check.py`) |
| Automated Testing + CI/CD | **External** | Workflows are complete but staged in `deploy/ci/`; the automation token lacks GitHub `workflows` permission (push rejected). Run `./scripts/enable-ci.sh` with a normal PAT to activate |
| Documentation set | Done | Architecture, API, Deployment, Security, Plugin Development, Voice, Device Integration, User Guide, Install, this audit |

## Known limitations (explicit)

1. **MariaDB/MySQL** is not a supported driver yet; PostgreSQL is the scale-out option.
2. **Matter and Tuya** have no native adapters; use MQTT/HTTP bridges.
3. **Email verification / email password reset** are not implemented.
4. **Native OPUS** in the browser voice loop is not shipped; PCM/WAV is used.
5. **CI workflows** must be enabled once with a token that has `workflows` permission.
6. A single node's device count is bounded by file descriptors/network; multi-node chat needs Redis/NATS and a shared DB, voice sessions are node-local.

Everything above is reproducible locally with `make test`, `make cover`, `make cover-gate`
and the load test in `tests/load`.
