# AIOS Design Details

Companion to [ARCHITECTURE.md](ARCHITECTURE.md): data model, deployment
topologies, performance budget, MVP, roadmap, risk register and test strategy.

## 1. ERD

```
users 1 ──< sessions 1 ──< messages
  │             │
  │             └──< memories (session-scoped)
  ├──< memories (long/semantic)
  ├──< devices
  ├──< api_keys
  └──< refresh_tokens

agents            routing data (code-owned builtins + custom)
personalities     preset (owner_id='') or user-owned
plugins           HTTP tool manifests
tasks             durable queue: reminders, automations, maintenance
usage_records     analytics + cost
audit_events      append-only security trail
settings          JSON config (global/user), holds encrypted secrets
```

Portability: TEXT ids, fixed-width UTC TEXT timestamps (lexicographic =
chronological in both engines), INTEGER booleans, TEXT JSON. One migration set
runs on SQLite and Postgres; `?` placeholders are rebound to `$n` for Postgres.
Repositories hide the dialect from the core.

Indexes: sessions(user_id, updated_at), messages(session_id, created_at),
memories(user_id, tier, created_at), memories(expires_at),
devices(token_prefix) unique, tasks(status, run_at), audit(created_at),
usage(user_id, created_at).

## 2. Folder structure

```
cmd/aios/                 server, migrate, seed, doctor, config, hash-password
internal/
  platform/               ids, typed errors, logging, retry, breaker, pagination
  domain/                 entities & value objects (pure Go)
  ports/                  repository + provider interfaces
  config/                 YAML/env config, defaults, validation
  security/               keyring, AES-GCM, JWT/refresh, RBAC, audit, limiter
  memory/                 three-tier memory + extraction
  personality/            presets + prompt composition
  agents/                 catalogue + routing
  brain/                  intent, context, tool loop, streaming, preview
  llm/                    router (retry/fallback/breaker/cost)
  tools/                  built-in tools + registry
  devices/                registration, commands, telemetry
  scheduler/              durable tasks → queue
  plugins/                manifest host + HTTP tools
  observability/          Prometheus, health, host stats
  app/                    composition root, bootstrap, jobs, lifecycle
  adapters/               sqlstore, cache, queue, vector, llm, mqtt, httpdev, httpx
  transport/httpapi/      REST, SSE, WebSocket, GraphQL, embedded dashboard
  voice/                  VAD, WAV/PCM, STT/TTS engines, conversation service
deploy/                   compose profiles, systemd, Prometheus rules
docs/                     architecture, API, security, voice, devices, plugins
tests/                    e2e + load
internal/.../static/      embedded dashboard (HTML/CSS/JS, no build step)
```

## 3. Performance budget (1 vCPU / 1 GB)

| Metric | Budget |
|---|---|
| Idle RSS | ≤ 90 MB |
| Memory ceiling | GOMEMLIMIT=512MiB, GOGC=60 (384MiB in Docker) |
| Cold start | ≤ 400 ms including fresh migrations |
| Chat overhead (mock model) | ≤ 10 ms p50 |
| SQLite | single writer, WAL, busy_timeout 5s, mmap 64 MB |
| Queue | 2 workers, bounded buffer, exponential backoff |
| Cache | LRU 10k entries, TTL 5 min |
| Vector search | brute force per user; 5k×384 ≈ 2M FLOPs/query |
| Body limits | 1 MB JSON, 32 MB audio |

Implemented optimisations: lazy provider construction, per-provider concurrency
semaphores, embedding cache, prepared statements, single-writer SQLite, context
deadlines everywhere, token-budgeted context, streaming, GOGC tuning.

## 4. MVP definition (shipped in v1.0)

1. Login/refresh with RBAC and bootstrap admin.
2. Streaming chat, sessions, personalities, agent selection.
3. Provider routing: local/Ollama/OpenAI/Anthropic/Gemini/DeepSeek/Mistral.
4. Three-tier memory with extraction and semantic recall.
5. Device registry, token auth, bounded commands, telemetry.
6. MQTT bridge, HTTP device transport, Home Assistant tool.
7. Voice loop (VAD/STT/TTS, wake word, barge-in) over WebSocket.
8. Plugin host with manifest validation and HTTP tools.
9. Reminders/automations on a durable task queue.
10. Admin dashboard, Prometheus metrics, health, audit log.
11. Docker/compose/systemd deployment, CI, ARM cross-builds.

## 5. Roadmap

| Phase | Scope |
|---|---|
| 1.1 | Keep coverage above 80% as the suite grows; OpenAPI-generated client; SSE reconnect |
| 1.2 | ✅ Postgres/Redis integration suites; queue visibility timeout + stale-task reclaim |
| 1.3 | ✅ Ed25519 plugin signing + WASM sandbox (wazero) + marketplace manifest |
| 1.4 | ✅ openWakeWord, Silero VAD bridge, per-user voice profiles; OPUS next |
| 1.5 | ✅ Authenticated MQTT chat bridge + real-broker tests; multi-instance sessions next |
| 1.6 | ✅ Document RAG (text/HTML/PDF ingest, chunk+embed, citations UI); OCR next |
| 1.7 | ✅ Per-user BYOK providers + OpenAI-compatible tunnel, self-registration, backup/restore, RTL dashboard, 2 GB profile |
| 2.0 | ✅ NATS event bus + cross-replica WebSocket fan-out and MQTT shared subs; service extraction next |

## 6. Risk register

| Risk | Impact | Mitigation |
|---|---|---|
| Weak hardware OOM | Crash/restart | GOMEMLIMIT/GOGC, bounded caches/queues, single-writer SQLite, load test |
| Provider outage | Failed chats | Retries, breakers, ordered fallbacks, always-available local provider |
| Prompt injection via pages | Unsafe actions | Capability-validated tools, bounded device commands, no shell tool, tool output treated as data |
| SSRF from web fetch | Network probing | Dial-time private-IP block, scheme allow-list, redirect cap, body caps |
| Plugin supply chain | Data exfiltration | Admin-only install, permissions, host allow-list, out-of-process HTTP |
| Secret leakage | Credential theft | AES-GCM at rest, env keys, log redaction, secrets never returned |
| SQLite write contention | Latency spikes | WAL, busy timeout, single writer, queue coalescing, Postgres path |
| Refresh token theft | Account takeover | Hashed rotation, family revocation on reuse, short access TTL |
| Voice misrecognition | Bad actions | Wake word, confirmations for destructive actions, capability bounds |
| Coverage gap | Regressions | Unit + E2E + load tests, CI gates; 80% tracked as release blocker |

## 7. Test strategy

- **Unit:** platform, config, security, memory extraction, intent, VAD/WAV,
  vector similarity, tool registry, SQL repositories.
- **Integration/E2E:** assembled app over HTTP with SQLite and mock
  model/voice/tools: login → chat (REST + SSE) → memory → device registration →
  telemetry → bounded command → RBAC denial → GraphQL → metrics.
- **Load:** `tests/load` drives a running instance with configurable concurrency,
  reports p50/p95/p99 and fails above a 5% error rate.
- **Gates:** `make cover` (atomic, `-coverpkg`) and `scripts/coverage-gate.sh`.
  Current measured total is **80.8%** with a de-duplicated merged profile; the
  gate enforces **80% overall and 80% per core package**.

## 8. Configuration precedence

1. built-in defaults tuned for 1 CPU / 1 GB;
2. `config.yaml` (strict decoding, `${ENV}` expansion);
3. `AIOS_*` environment variables (highest priority);
4. validation at boot (fail fast on unsafe production settings).

Secrets never belong in YAML: use env vars or the encrypted secret store. Keys
live in `data/keys` with 0600 permissions when auto-generation is enabled.

