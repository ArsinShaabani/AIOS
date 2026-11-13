# Install

AIOS ships as a **single static binary** (no cgo, no runtime dependencies) and
includes the admin dashboard, REST/SSE/WebSocket/GraphQL APIs, voice, devices,
plugins and document RAG.

## One-command install (recommended)

On a fresh Linux server (amd64 or arm64), as root:

```bash
curl -fsSL https://raw.githubusercontent.com/ArsinShaabani/AIOS/main/deploy/install.sh \
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

## Behind nginx (TLS)

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

## Uninstall

```bash
sudo ./deploy/install.sh --uninstall          # keeps data
sudo ./deploy/install.sh --uninstall --purge  # removes data + user
```