# AIOS API

Base URL: `http://<host>:8080`. All bodies are JSON. Authenticate with
`Authorization: Bearer <access token>` or `X-API-Key: aios_…`. Device firmware uses
its device token as a bearer token on `/api/v1/device/*`.

The machine-readable specification is served at [`/openapi.yaml`](http://localhost:8080/openapi.yaml).
GraphQL is at `POST /graphql` (playground at `GET /graphql`). Prometheus metrics
at `GET /metrics`; liveness `GET /healthz`; readiness `GET /readyz`.

## Authentication

| Method | Path | Notes |
|---|---|---|
| POST | `/api/v1/auth/login` | `{email,password}` → `{tokens,user,permissions}` |
| POST | `/api/v1/auth/refresh` | `{refresh_token}` → rotated pair |
| POST | `/api/v1/auth/logout` | revokes the refresh token |
| GET | `/api/v1/auth/me` | current user, principal, permissions |
| GET/POST | `/api/v1/auth/api-keys` | list/create API keys (token shown once) |
| DELETE | `/api/v1/auth/api-keys/{id}` | revoke |

Access tokens live 15 minutes by default; refresh tokens 30 days and rotate on
every use. Reusing a rotated token revokes the whole family.

```bash
curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@aios.local","password":"..."}'
```

## Chat

`POST /api/v1/chat`

```json
{"message":"turn on the kitchen light","session_id":"ses_…","agent_id":"agent_home","stream":false}
```

Response (non-stream) contains `session_id`, `message_id`, `content`, `agent_id`,
`provider`, `model`, `usage`, `tool_calls`, `latency_ms`, `intent`.

Streaming: set `"stream": true` or `Accept: text/event-stream`. Each SSE frame is
`data: {"type": "...", ...}` with types `session`, `delta`, `tool_start`,
`tool_result`, `done`, `error`.

WebSocket: `GET /api/v1/ws/chat?token=<jwt>`; send `{"type":"message","message":…,
"session_id":…}`; receive the same event objects. Send `{"type":"cancel"}` for
barge-in. One turn runs at a time per connection.

Sessions are created implicitly by chat. List them with `GET /api/v1/sessions`,
then `GET/PATCH/DELETE /api/v1/sessions/{id}` and
`GET /api/v1/sessions/{id}/messages?limit=100`.

## Agents and personalities

- `GET /api/v1/agents?all=true` → agents + tool catalogue.
- `POST /api/v1/agents`, `PUT /api/v1/agents/{id}`, `DELETE /api/v1/agents/{id}`
  (requires `agents:write`). Built-in agents can be disabled but not deleted.
- `GET /api/v1/personalities`; `POST`, `PUT /{id}`, `DELETE /{id}`. Presets are
  protected; users can create their own tone/humour/length profiles.

## Memory

- `GET /api/v1/memories?tier=long&kind=fact&q=tea&limit=50`
- `POST /api/v1/memories` → `{content,kind,importance,tags,pinned}`
- `DELETE /api/v1/memories/{id}`; `DELETE /api/v1/memories` wipes all (GDPR).
- Admin: `GET /api/v1/admin/memories?user_id=…`.

Memories are extracted automatically from conversations (English + Farsi) and
embedded with the configured embedder (local hash embeddings by default).

## Devices

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/devices` | list (filters: kind, status, room, q) |
| POST | `/api/v1/devices` | register → returns a one-time device token |
| GET/PATCH/DELETE | `/api/v1/devices/{id}` | manage |
| POST | `/api/v1/devices/{id}/command` | `{capability,value}` — validated against safety bounds |
| GET | `/api/v1/devices/{id}/telemetry` | latest cached values |
| POST | `/api/v1/devices/{id}/rotate-token` | new credential |
| POST | `/api/v1/device/telemetry` | device auth: `{metric,value,unit}` → 202 |
| POST | `/api/v1/device/heartbeat` | device auth: marks online |
| GET | `/api/v1/device/me` | device auth: own record |

## Voice

- `GET /api/v1/voice/config` → providers, sample rate, VAD, wake words.
- `POST /api/v1/voice/transcribe` — multipart `audio` (WAV; resampled to 16 kHz).
- `POST /api/v1/voice/synthesize` — `{text,voice?,format?}` → audio bytes.
- `GET /api/v1/ws/voice?token=…` — binary PCM16 16 kHz frames in; JSON events and
  binary audio out. Events: `ready`, `speech_start`, `speech_end`, `transcript`,
  `reply`, `audio`, `audio_end`, `wake_needed`, `cancelled`, `error`.

## Models and usage

- `GET /api/v1/models` → providers (health, breaker, models), defaults, routing.
- `GET /api/v1/usage?from=…&to=…&all=true` → totals.
- `GET /api/v1/usage/daily`, `GET /api/v1/usage/providers`.
- `GET /api/v1/admin/queue` → queue + durable task counts.
- `GET /api/v1/tasks?status=pending`; `DELETE /api/v1/tasks/{id}` cancels.

## Plugins

- `GET /api/v1/plugins`
- `POST /api/v1/plugins` with a manifest:

```json
{
  "name": "weather", "version": "1.0.0", "description": "Weather tools",
  "endpoint": "http://127.0.0.1:7070", "permissions": ["internet"],
  "tools": [{
    "name": "weather_now", "description": "Current weather",
    "method": "POST", "path": "/tools/weather_now",
    "parameters": {"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}
  }]
}
```

- `PATCH /api/v1/plugins/{id}` `{enabled}`; `DELETE /api/v1/plugins/{id}`.

WASM plugins use the same endpoint with `"kind": "wasm"` plus
`"wasm_module": "<base64 .wasm>"`; signed artifacts also carry `signature`,
`wasm_sha256` and `wasm_size`. When `plugins.require_signature` is enabled, only
manifests signed by a key in `plugins.trusted_keys` are accepted.

## Settings, admin and errors

- `GET /api/v1/settings?scope=global|user`; `PUT/DELETE /api/v1/settings/{key}`.
- Admin users: `GET/POST /api/v1/admin/users`, `PATCH/DELETE /{id}`,
  `POST /{id}/reset-password`, `POST /{id}/revoke-tokens`.
- `GET /api/v1/admin/overview`, `/admin/system`, `/admin/audit`,
  `/admin/prompt/{sessionID}` (exact rendered prompt), `/admin/maintenance/prune`.
- Errors use `{"error":{"code","message","trace_id"}}` with 4xx/5xx status and
  never expose internal causes.

## GraphQL

```graphql
query {
  me { email role }
  agents { name tools }
  sessions(limit: 5) { title messageCount }
  memories(query: "tea", limit: 3) { kind content }
  providers { name healthy models }
}
mutation {
  sendMessage(message: "hello", sessionId: null) { content provider }
}
```

Properties are camelCase in GraphQL (`displayName`, `messageCount`, `agentId`).
