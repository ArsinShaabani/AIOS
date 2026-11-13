# Deployment

AIOS ships as a single static binary. This guide covers the edge profile (1 CPU /
1 GB), Docker, systemd, the full stack, upgrades and troubleshooting.

## 1. Requirements

- Linux (amd64, arm64, armv7), no cgo, no GPU
- 256 MB RAM minimum; 512 MB–1 GB recommended for voice
- Optional: Ollama (local LLM), faster-whisper (STT), Piper/Edge-TTS (TTS),
  Mosquitto (MQTT), Home Assistant, Postgres/Redis/Qdrant for scale-out

## 2. Quick start (binary)

```bash
git clone https://github.com/ArsinShaabani/AIOS.git && cd AIOS
grep -v '^#' config.example.yaml > config.yaml   # or copy and edit
go build -trimpath -ldflags "-s -w" -o aios ./cmd/aios
./aios doctor            # validates config and dependencies
./aios server            # http://127.0.0.1:8080
```

First boot creates the database, presets, agents and an administrator. If
`security.bootstrap.password` is empty a random password is printed **once** in
the log; change it after login.

Point a real model at it (fully offline):

```bash
ollama pull qwen2.5:1.5b
AIOS_LLM_DEFAULT_PROVIDER=ollama AIOS_LLM_DEFAULT_MODEL=qwen2.5:1.5b ./aios server
```

## 3. Docker

```bash
export AIOS_ADMIN_PASSWORD='a-long-unique-password'
cp config.example.yaml config.yaml
docker compose up -d --build
docker compose logs -f aios
```

The default compose profile caps the container at 512 MB / 1 CPU, runs read-only
with a tmpfs, drops privileges and stores data in the `aios-data` volume.
Health checks call `aios doctor` (config + database + vector + queue).

Multi-arch image:

```bash
docker buildx build --platform linux/amd64,linux/arm64,linux/arm/v7 -t ghcr.io/<you>/aios:1.0.0 --push .
```

## 4. systemd

```bash
sudo useradd --system --home /var/lib/aios aios
sudo install -D -m 0755 aios /usr/local/bin/aios
sudo install -D -m 0644 deploy/systemd/aios.service /etc/systemd/system/aios.service
sudo install -D -m 0640 config.yaml /etc/aios/config.yaml
sudo mkdir -p /var/lib/aios && sudo chown aios:aios /var/lib/aios
sudo systemctl daemon-reload && sudo systemctl enable --now aios
journalctl -u aios -f
```

The unit applies `NoNewPrivileges`, `ProtectSystem=strict`, a syscall filter and
`GOMEMLIMIT=384MiB`.

## 5. Raspberry Pi / ARM

```bash
make release    # dist/aios-1.0.0-linux-armv7 + arm64 + amd64, sha256 checksums
scp dist/aios-1.0.0-linux-armv7 pi@host:/tmp/
```

Recommended Pi settings: 64-bit OS for arm64, `gpu_mem=16`, swap 512 MB, and
`memory.max_tokens: 512`, `memory.retrieval_top_k: 3`, `queue.workers: 1` in
config. A 1.5B quantised Ollama model is the practical ceiling on a Pi 4.

## 6. Event bus (NATS) for multi-instance deployments

The in-memory bus is ideal for a single box. When several AIOS processes share
work, point them at NATS so chat/device/reminder events are exchanged reliably:

```yaml
events:
  driver: nats
  url: nats://127.0.0.1:4222
  subject_prefix: aios.events
  timeout: 5s
```

NATS reconnects automatically; `/readyz` reports `events` as degraded when the
connection is down. Redis remains responsible for cache/queue, NATS only carries
events.

## 7. Full stack (larger host)

`deploy/docker-compose.full.yml` starts Postgres, Redis, Qdrant, Ollama,
faster-whisper and a Wyoming Piper container, then wires AIOS to them:

```bash
export AIOS_ADMIN_PASSWORD='...'
export POSTGRES_PASSWORD='...'
docker compose -f deploy/docker-compose.full.yml up -d
```

Notes:
- Whisper image exposes port 9000 internally; AIOS calls `http://whisper:9000/v1`.
- Wyoming Piper uses a different protocol from the Piper HTTP wrapper expected by
  `piper_http`; either run a Piper HTTP bridge or switch `voice.tts.provider` to
  `edge_http`/`openai` in this profile.
- Postgres migrations are the same files; no code changes are required.

## 8. Reverse proxy and TLS

Terminate TLS at Caddy/Nginx and proxy to `127.0.0.1:8080`. Requirements:
- forward `X-Forwarded-For` (trusted only from loopback by AIOS);
- disable buffering for `/api/v1/chat` (SSE) and WebSocket upgrade headers;
- allow large bodies for `/api/v1/voice/transcribe` (32 MB).

Caddy example:

```
aios.example.com {
  reverse_proxy 127.0.0.1:8080 {
    flush_interval -1
  }
}
```

In production set `server.cors_origins` to the exact origin, `server.tls` or a
proxy for HTTPS, and a 32+ byte `security.jwt_secret` (or rely on auto-generated
keys in `data/keys`).

## 9. Backups and upgrades

- SQLite: `sqlite3 data/aios.db ".backup '/backup/aios-$(date +%F).db'"` (WAL-safe).
- Postgres: `pg_dump -Fc aios > backup.dump`.
- Back up `data/keys` separately; losing the encryption key makes stored secrets
  unreadable (by design).
- Upgrade: stop, replace binary/image, start. Migrations run automatically and
  are forward-only. Tagged releases publish checksums and multi-arch images.

## 10. Operations checklist

| Task | Command / endpoint |
|---|---|
| Health | `GET /healthz`, `GET /readyz` |
| Metrics | `GET /metrics` (scrape with `deploy/prometheus`) |
| Diagnostics | `aios doctor` |
| Effective config | `aios config` |
| Migrations only | `aios migrate` |
| Seed presets/admin | `aios seed` |
| Queue/tasks | dashboard → Task Queue, `GET /api/v1/admin/queue` |
| Prune memories/audit | `POST /api/v1/admin/maintenance/prune` |
| Rotate a device token | `POST /api/v1/devices/{id}/rotate-token` |
| Revoke sessions | `POST /api/v1/admin/users/{id}/revoke-tokens` |

## 11. Troubleshooting

| Symptom | Fix |
|---|---|
| `invalid configuration` | run `aios doctor`; production requires secrets/CORS tightening |
| 503 on `/readyz` | check the failing check in the JSON body (db/cache/vector/queue) |
| Chat always uses the template reply | `llm.default_provider` is still `local`; enable Ollama/OpenAI |
| Slow first token | model cold start or network; check `/metrics` LLM latency |
| Voice silent | verify STT/TTS URLs from inside the container, then `POST /voice/synthesize` |
| Devices offline | token mismatch, MQTT prefix, or `command_url` unreachable |
| High memory | lower `cache.max_entries`, `llm.max_concurrency`, `memory.retrieval_top_k` |
