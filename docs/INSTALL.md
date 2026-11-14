# Install

AIOS ships as a **single static binary** (no cgo, no runtime dependencies) and
includes the admin dashboard, REST/SSE/WebSocket/GraphQL APIs, voice, devices,
plugins and document RAG.

## One-command install (recommended)

On a fresh Linux server (amd64 or arm64), as root:

```bash
curl -fsSL https://github.com/ArsinShaabani/AIOS/releases/latest/download/install.sh \
  | sudo bash -s -- --admin-password 'Choose-A-Strong-Password'
```

The installer:

1. detects the architecture and downloads the latest release from GitHub
   (verifying the published SHA-256),
2. creates the `aios` system user, `/opt/aios`, `/var/lib/aios`, `/etc/aios`,
3. writes a production `config.yaml` (loopback-safe CORS, rate limits on),
4. installs and starts a hardened `aios.service` (systemd),
5. waits for `/healthz`, opens the port in ufw/firewalld when present,
6. prints the dashboard URL and credentials (also stored in
   `/etc/aios/admin-password.txt`, mode 0600).

Useful flags:

```bash
--port 8080            # HTTP port (auto-picks a free one if busy)
--bind 0.0.0.0         # listen address
--admin-email ops@example.com
--provider ollama --model qwen2.5:1.5b   # configure a real model at install time
--version v1.0.0       # pin a release
--local-binary ./aios  # install a locally built binary (no download)
--no-systemd           # install files only
--uninstall [--purge]  # remove the service (and optionally data)
```

Verify:

```bash
systemctl status aios
curl -s localhost:8080/healthz
curl -s localhost:8080/readyz | head -c 400
journalctl -u aios -f
```

## After install

1. Open `http://<server-ip>:8080/` and sign in with the printed credentials.
2. Change the password (Users → reset password) and set your timezone.
3. Pick a model:
   - **offline:** `ollama pull qwen2.5:1.5b`, then set
     `llm.default_provider: ollama` / `llm.default_model: qwen2.5:1.5b` in
     `/etc/aios/config.yaml` and `systemctl restart aios`;
   - **cloud:** enable a provider block and set `AIOS_*` env vars.
4. Optional engines (voice, NATS, Redis, Qdrant, Postgres) live in
   `deploy/docker-compose.full.yml`.

## Upgrading an existing install

```bash
curl -fsSL https://github.com/ArsinShaabani/AIOS/releases/latest/download/install.sh -o /tmp/aios-install.sh
sudo bash /tmp/aios-install.sh --upgrade
```

`--upgrade` replaces the binary and restarts the service; it leaves
`/etc/aios/config.yaml`, the database and plugin data untouched. Without the
flag the installer also keeps an existing config, and it never blanks the
administrator password when `--admin-password` is omitted.

If the installer itself is unusable on a host, replace the binary directly:

```bash
VER=v1.0.5
curl -fsSL "https://github.com/ArsinShaabani/AIOS/releases/download/$VER/aios-linux-amd64" -o /tmp/aios
sudo install -m 0755 /tmp/aios /usr/local/bin/aios
sudo systemctl restart aios
aios version
```

## Diagnosing the model layer

If chat answers with the built-in template text ("I'm running AIOS' built-in
local template model..."), the request fell back because the configured
provider failed. AIOS now reports this instead of hiding it: the API returns
`degraded: true`, a `notice` naming both providers, and the chat UI shows a
warning next to the message.

Run the diagnostics as the service user:

```bash
sudo -u aios /usr/local/bin/aios llm test -config /etc/aios/config.yaml
```

It builds every configured provider, pings each one and sends a real
completion, printing the exact URL and error, for example:

```
provider ollama (models: qwen2.5:1.5b)
  ✗ ping failed: ollama.network: provider unreachable:
      Get "http://127.0.0.1:11434/v1/models": dial tcp 127.0.0.1:11434: connect: connection refused
```

| Symptom | Cause | Fix |
| --- | --- | --- |
| `connect: connection refused` | Ollama is not running | `systemctl status ollama` / `ollama serve` |
| `model not found` | model not pulled | `ollama pull qwen2.5:1.5b` |
| `404` on `/v1/chat/completions` | AIOS older than v1.0.3 | upgrade |
| first token takes minutes | model too large for RAM | pick a smaller model |

## Local model (Ollama)

AIOS ships a deterministic template model so the API works without credentials.
For real answers install Ollama and point AIOS at it:

```bash
curl -fsSL https://raw.githubusercontent.com/ArsinShaabani/AIOS/main/deploy/setup-ollama.sh -o /tmp/setup-ollama.sh
sudo bash /tmp/setup-ollama.sh                 # picks a model that fits the RAM
sudo bash /tmp/setup-ollama.sh --model qwen2.5:3b
sudo bash /tmp/setup-ollama.sh --status
```

The script installs Ollama, pulls one model, rewrites only the `llm:` section of
`/etc/aios/config.yaml` (backup kept, owner/mode preserved), restarts AIOS and
verifies the model answers before reporting success.

Model guidance for CPU-only hosts (no GPU). Throughput is per core and only a
rough guide — benchmark on your own host with `aios llm test`, which prints
tok/s for each provider:

| RAM | Model | Size | CPU speed (4 vCPU) | Notes |
| --- | --- | --- | --- | --- |
| 1–3 GB | `qwen2.5:0.5b` | ~400 MB | ~15–30 tok/s | fastest usable tier; short answers |
| 1–3 GB | `gemma3:270m` | ~300 MB | ~25–40 tok/s | smallest; very terse |
| 2–4 GB | `llama3.2:1b` | ~1.3 GB | ~8–15 tok/s | better English, weaker Persian |
| 2–6 GB | `qwen2.5:1.5b` | ~1 GB | ~5–10 tok/s | previous default; noticeably slower |
| 4–8 GB | `qwen2.5:3b` | ~2 GB | ~3–6 tok/s | better reasoning |
| 8 GB+ | `qwen2.5:7b` | ~4.7 GB | ~1–3 tok/s | avoid without a GPU |

Switching models takes one command (no reinstall):

```bash
sudo ollama pull qwen2.5:0.5b
sudo bash /tmp/setup-ollama.sh --model qwen2.5:0.5b   # rewrites llm: and restarts
```

The setup script also writes an Ollama systemd drop-in that matters more than
the model choice for perceived latency:

| Setting | Value | Why |
| --- | --- | --- |
| `OLLAMA_KEEP_ALIVE` | `-1` | model stays in RAM; no reload per message |
| `OLLAMA_NUM_PARALLEL` | `1` | one generation already saturates a small CPU |
| `OLLAMA_MAX_LOADED_MODELS` | `1` | prevents RAM thrashing |
| `OLLAMA_CONTEXT_LENGTH` | 2048–8192 | attention cost grows with context |
| `OLLAMA_KV_CACHE_TYPE` | `q8_0` | halves KV cache memory |

Ollama exposes an OpenAI-compatible API under `/v1`; AIOS normalizes
`base_url` automatically, so `http://127.0.0.1:11434` is enough. Keep
`max_concurrency: 1` on a single-core box: one generation already saturates the
CPU, and parallel requests only add latency and memory pressure. For very small
hosts you can also lower `memory.max_context_tokens` (default 3000) and
`llm.max_tokens` to trim the prompt.

Streaming, tools and memory work the same as with a hosted provider. Models
below ~3B follow tool calls unreliably; keep agents simple on very small hosts.

## Publish behind nginx (script)

`deploy/setup-nginx.sh` writes a working vhost, chooses a certificate that
exists, wires the WebSocket/SSE locations and aligns `public_url` /
`cors_origins` in the AIOS config.

```bash
# real domain (Cloudflare or direct) — requests a certbot certificate
sudo bash setup-nginx.sh --domain aios.example.com

# no domain yet: self-signed certificate for the server IP
sudo bash setup-nginx.sh --ip --default-server
```

Then AIOS answers on `https://<domain-or-ip>/`. Notes:

- `--ip` produces a self-signed certificate: the browser warns once. A domain is
  the only way to get a warning-free HTTPS URL.
- `--default-server` makes this vhost answer for bare IPs; omit it when another
  site already owns `default_server`.
- The script refuses to use a certificate for a different hostname, backs up the
  previous vhost, and only reloads nginx when `nginx -t` passes.
- Cloudflare: keep the proxy ON, SSL mode **Full (strict)**, WebSockets enabled.

## Behind nginx (manual configuration)

The dashboard uses absolute asset paths, so serve it on its **own hostname**
rather than a sub-path:

```nginx
server {
  listen 443 ssl http2;
  server_name aios.example.com;
  ssl_certificate     /etc/letsencrypt/live/aios.example.com/fullchain.pem;
  ssl_certificate_key /etc/letsencrypt/live/aios.example.com/privkey.pem;

  client_max_body_size 32m;          # voice + document uploads

  location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;   # WebSocket (chat/voice)
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_read_timeout 3600s;
    proxy_buffering off;                       # SSE streaming
  }
}
```

After adding TLS, set `server.cors_origins` to the public origin (e.g.
`https://aios.example.com`) if you call the API from another web app.

## Resource profile

| Host | Notes |
|---|---|
| 1 vCPU / 1 GB | default config; `GOMEMLIMIT=384MiB`, 2 queue workers |
| Raspberry Pi 4 | use the arm64 binary; 1.5B local model is the practical ceiling |
| 4 GB+ | raise `queue.workers`, `cache.max_entries`, `llm.max_concurrency` |

## Manual / offline install

```bash
make build                 # local binary
sudo ./deploy/install.sh --local-binary ./aios --admin-password '...'
```

Air-gapped hosts: copy the release asset (`aios-linux-amd64` / `-arm64`) and its
`.sha256`, then use `--local-binary`.

## Troubleshooting

### `production requires security.bootstrap.password with >=12 chars` (crash loop)

Production mode rejects bootstrap passwords shorter than 12 characters. The
installer now refuses them up front; on an existing install either rotate the
password or use a longer one:

```bash
# rotate to a 12+ character password (also fixes permissions)
curl -fsSL https://github.com/ArsinShaabani/AIOS/releases/latest/download/install.sh -o /tmp/aios.sh
sudo bash /tmp/aios.sh --repair --admin-password 'a-long-new-password'
```

### `permission denied` reading `/etc/aios/config.yaml` (crash loop)

The service runs as the unprivileged `aios` user, so the config must be readable
by that user's group. Fixed installs look like this:

```
drwxr-x--- root aios /etc/aios
-rw-r----- root aios /etc/aios/config.yaml
```

Repair an existing install (no download, no data loss):

```bash
curl -fsSL https://github.com/ArsinShaabani/AIOS/releases/latest/download/install.sh \
  | sudo bash -s -- --repair
```

Or fix it manually:

```bash
sudo bash -c 'g=$(id -gn aios); install -d -m 0750 -o root -g "$g" /etc/aios; \
  chown root:"$g" /etc/aios/config.yaml; chmod 0640 /etc/aios/config.yaml; \
  install -d -m 0750 -o aios -g "$g" /var/lib/aios; \
  systemctl reset-failed aios; systemctl restart aios; sleep 3; \
  systemctl is-active aios; journalctl -u aios -n 15 --no-pager'
```

### Service starts then exits

```bash
sudo journalctl -u aios -n 50 --no-pager
sudo -u aios /usr/local/bin/aios doctor -config /etc/aios/config.yaml
```

`doctor` prints the failing check (database, config, vector, queue, providers).

### Dashboard unreachable from outside

The port is open locally (`curl -s localhost:8080/healthz`) but not from the
internet: allow it in the provider firewall/security group, or front it with
nginx on 443 (see the nginx section above).

## Uninstall

```bash
# keeps data and config
sudo bash -s -- --uninstall </dev/null   # via fetched install.sh
sudo ./deploy/install.sh --uninstall     # via a local checkout

# also removes /var/lib/aios, /etc/aios and the aios user
sudo ./deploy/install.sh --uninstall --purge
```
