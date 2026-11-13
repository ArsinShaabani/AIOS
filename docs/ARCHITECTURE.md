# AIOS Architecture

> **Status:** v1.0, runnable core. Requirements, C4 model, module boundaries and
> request flow live here; data model, performance budget, roadmap, MVP and risks
> are in [DESIGN.md](DESIGN.md).

## 1. Requirements analysis

| Requirement | Design response |
|---|---|
| 1 CPU / 1 GB RAM, no GPU | Single Go binary, pure-Go SQLite, bounded caches/queues, GOMEMLIMIT/GOGC defaults, no JVM/Python in the hot path |
| Microservice-oriented | Hexagonal modules with port/adapter seams; every module is extractable without changing core code |
| Thousands of devices | MQTT fan-out (paho), stable topic contracts, presence cache, stateless HTTP, Redis option for shared state |
| Robot/IoT/smart-home | Capability validation with safety bounds, HTTP/MQTT transports, Home Assistant REST, rosbridge guidance |
| Voice | VAD + STT/TTS ports, streaming WebSocket loop, barge-in, wake words |
| Any LLM | Adapters for OpenAI-compatible, Anthropic, Gemini, Ollama, local; router with fallback and budgets |
| Personality + memory | Personality engine composes prompts; 3-tier memory with local semantic embeddings |
| Plugins | Manifest-based HTTP tools, out-of-process, permission labels, host allow-list |
| Enterprise security | JWT + refresh rotation with reuse detection, RBAC, audit log, AES-256-GCM at rest, SSRF guard, rate limits |
| Enterprise UX | Embedded admin SPA (no build step), SSE + WebSocket chat, voice UI, usage/cost analytics |

**Non-goals (v1):** distributed training, multi-tenant billing, native mobile
apps, WASM plugin sandbox, full Matter controller. These are roadmap items, not
silent omissions.

## 2. Decision: modular monolith first

Twelve microservices cannot fit in 1 GB. AIOS is a **modular monolith with
hexagonal boundaries**:

- every bounded context owns its package, ports and repositories;
- `internal/app` is the only composition root;
- transports never call adapters directly;
- cross-module communication uses typed ports and an in-process event bus.

To extract a module: implement its port over RPC/HTTP, point the interface at the
remote adapter, and deploy it separately. `deploy/docker-compose.full.yml` already
splits Postgres, Redis, Qdrant, Ollama, Whisper and Piper from the core.

## 3. C4 — context

```
User (browser, mobile, voice)
        │ HTTPS / WS
        ▼
┌──────────────────────────── AIOS ────────────────────────────┐
│ Gateway (REST, SSE, WS, GraphQL, MQTT bridge)                │
│   │ authn/authz · rate limit · validation · audit            │
│   ▼                                                          │
│ Brain (intent → agent → route → context → tool loop → reply) │
│   │                    │                    │                │
│   ▼                    ▼                    ▼                │
│ Memory              Agents/Tools        Model Router         │
│ (short/long/vector) (search, devices…)  (providers, fallback)│
│   │                    │                    │                │
│   ▼                    ▼                    ▼                │
│ SQLite/Postgres      MQTT/HTTP/ROS      OpenAI/Anthropic/    │
│ Vector/Qdrant        Home Assistant     Gemini/Ollama/local  │
└──────────────────────────────────────────────────────────────┘
```

## 4. Module map

| Module | Responsibility | Key ports |
|---|---|---|
| `transport/httpapi` | REST, SSE, WebSocket chat/voice, GraphQL, UI | — |
| `brain` | intent, context builder, tool loop, streaming, preview | repositories, `ModelRouter`, `ToolRegistry` |
| `agents` | data-driven agent catalogue + selection | `AgentRepository` |
| `memory` | remember/recall/extract/prune, semantic warm-up | `MemoryRepository`, `VectorStore`, `Embedder` |
| `personality` | presets and prompt composition | `PersonalityRepository` |
| `llm` | routing, retries, breakers, cost, embedding cache | `Provider` |
| `tools` | web, knowledge, devices, memory, scheduler, HA, email, navigation, MQTT | `Tool` |
| `devices` | registration, token auth, capability validation, telemetry | `DeviceRepository`, `DeviceTransport` |
| `scheduler` | durable tasks → queue jobs | `TaskRepository` |
| `plugins` | manifest validation, HTTP tool execution | `PluginRepository` |
| `security` | keys, AES-GCM, JWT/refresh, RBAC, audit, rate limits | `TokenRepository` |
| `observability` | Prometheus, health, host stats | — |
| adapters | SQL, cache, queue, vector, LLM, MQTT, HTTP devices | implement ports |


## 5. Request flow (chat)

1. authenticate JWT/API key → principal + user;
2. load or create session (ownership enforced);
3. persist the user turn;
4. rule-based intent detection (no model call);
5. agent selection by keyword/intent score (an explicit agent wins);
6. router resolves provider/model + fallbacks from routing rules;
7. context builder: personality + agent prompt + relevant memories + recent
   history, trimmed to `memory.max_context_tokens`;
8. model call (stream or blocking); tool calls execute with timeouts and
   capability validation; results feed the next iteration (default max 4);
9. persist assistant/tool messages, update usage/cost, publish `chat.completed`;
10. enqueue `memory.extract` (rule-based facts, English + Farsi).

Tool failures return to the model as `is_error` results and are never silently
swallowed. If a stream fails after emitting content, the error is surfaced rather
than duplicating text.

## 6. Extension points

- **Provider:** implement `ports.Provider`, add a factory case. Core untouched.
- **Tool:** implement `ports.Tool`, register it; agents allow-list it by name.
- **Plugin:** HTTP manifest with JSON-Schema tools; no recompile.
- **Transport:** implement `DeviceTransport` (MQTT, HTTP, rosbridge, TCP…).
- **Store:** implement repository ports; SQL variants share migrations.
- **Event consumer:** subscribe to `chat.completed`, `device.telemetry`,
  `reminder.fired`; the bus is swappable between in-memory and NATS without
  touching producers.

## 7. Observability

Prometheus metrics: HTTP count/latency, LLM requests/latency/tokens/cost,
fallbacks, chat and tool counts, queue depth/running, devices by status, memory
records/vectors, voice sessions. Health endpoints: `/healthz` (liveness) and
`/readyz` (database, cache, vector, queue). The dashboard reads the same data
through `/api/v1/admin/*`.
