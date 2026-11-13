#!/usr/bin/env bash
# AIOS one-command installer for Linux (amd64 / arm64).
#
#   curl -fsSL https://raw.githubusercontent.com/ArsinShaabani/AIOS/main/deploy/install.sh \
#     | sudo bash -s -- --admin-password 'CHANGE_ME_STRONG'
#
# Flags:
#   --admin-password <pw>   bootstrap administrator password (default: generated)
#   --admin-email <email>   default admin@aios.local
#   --port <port>           HTTP port to listen on (default 8080)
#   --bind <addr>           listen address (default 0.0.0.0)
#   --version <tag>         release tag to install (default: latest)
#   --local-binary <path>   install from a local binary instead of GitHub
#   --provider <name>       default LLM provider (default local; e.g. ollama)
#   --model <name>          default model (e.g. qwen2.5:1.5b)
#   --no-systemd            install but do not start a service
#   --uninstall             stop and remove AIOS (keeps /var/lib/aios unless --purge)
set -euo pipefail

REPO="ArsinShaabani/AIOS"
APP_USER="aios"
PREFIX="/opt/aios"
DATA_DIR="/var/lib/aios"
CONF_DIR="/etc/aios"
BIN="/usr/local/bin/aios"
SERVICE="/etc/systemd/system/aios.service"

ADMIN_EMAIL="admin@aios.local"
ADMIN_PASSWORD=""
PORT="8080"
BIND="0.0.0.0"
VERSION=""
LOCAL_BINARY=""
PROVIDER="local"
MODEL="local-template"
USE_SYSTEMD=1
UNINSTALL=0
PURGE=0

log()  { printf '\033[1;34m[aios]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[aios]\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31m[aios]\033[0m %s\n' "$*" >&2; exit 1; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --admin-password) ADMIN_PASSWORD="${2:-}"; shift 2 ;;
    --admin-email)    ADMIN_EMAIL="${2:-}"; shift 2 ;;
    --port)           PORT="${2:-}"; shift 2 ;;
    --bind)           BIND="${2:-}"; shift 2 ;;
    --version)        VERSION="${2:-}"; shift 2 ;;
    --local-binary)   LOCAL_BINARY="${2:-}"; shift 2 ;;
    --provider)       PROVIDER="${2:-}"; shift 2 ;;
    --model)          MODEL="${2:-}"; shift 2 ;;
    --no-systemd)     USE_SYSTEMD=0; shift ;;
    --uninstall)      UNINSTALL=1; shift ;;
    --purge)          PURGE=1; shift ;;
    -h|--help)        sed -n '2,20p' "$0"; exit 0 ;;
    *) die "unknown flag: $1" ;;
  esac
done
[[ $EUID -eq 0 ]] || die "run as root (sudo)"

if [[ $UNINSTALL -eq 1 ]]; then
  log "stopping and removing AIOS"
  systemctl disable --now aios >/dev/null 2>&1 || true
  rm -f "$SERVICE" "$BIN"
  systemctl daemon-reload || true
  if [[ $PURGE -eq 1 ]]; then
    rm -rf "$DATA_DIR" "$CONF_DIR" "$PREFIX"
    id -u "$APP_USER" >/dev/null 2>&1 && userdel "$APP_USER" 2>/dev/null || true
    log "purged data, config and user"
  else
    log "kept $DATA_DIR and $CONF_DIR (use --purge to delete)"
  fi
  exit 0
fi

case "$(uname -m)" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) die "unsupported architecture $(uname -m); build from source with 'make build'" ;;
 esac
install_pkgs() {
  if command -v apt-get >/dev/null; then
    DEBIAN_FRONTEND=noninteractive apt-get update -qq || warn "apt update failed; continuing"
    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq curl ca-certificates tar || die "cannot install dependencies"
  elif command -v dnf >/dev/null; then
    dnf install -y -q curl ca-certificates tar || die "cannot install dependencies"
  elif command -v apk >/dev/null; then
    apk add --no-cache curl ca-certificates tar || die "cannot install dependencies"
  fi
}
command -v curl >/dev/null && command -v tar >/dev/null || install_pkgs

if ! id -u "$APP_USER" >/dev/null 2>&1; then
  log "creating system user $APP_USER"
  if ! useradd --system --home-dir "$DATA_DIR" --shell /usr/sbin/nologin "$APP_USER" 2>/dev/null; then
    useradd --system --home-dir "$DATA_DIR" --shell /sbin/nologin "$APP_USER"
  fi
fi
install -d -m 0750 -o "$APP_USER" -g "$APP_USER" "$DATA_DIR"
# Config is owned by root but group-readable by the service user.
install -d -m 0750 -o root -g "$APP_USER" "$CONF_DIR"
port_busy() {
  if command -v ss >/dev/null; then
    ss -ltn 2>/dev/null | awk '{print $4}' | grep -q ":$1$"
  elif command -v netstat >/dev/null; then
    netstat -ltn 2>/dev/null | awk '{print $4}' | grep -q ":$1$"
  else
    return 1
  fi
}
if port_busy "$PORT"; then
  for candidate in 8081 8082 8090 9080; do
    if ! port_busy "$candidate"; then
      warn "port $PORT is busy; switching to $candidate"
      PORT="$candidate"
      break
    fi
  done
  port_busy "$PORT" && die "no free port found; pass --port <port>"
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

if [[ -n "$LOCAL_BINARY" ]]; then
  [[ -f "$LOCAL_BINARY" ]] || die "local binary not found: $LOCAL_BINARY"
  install -m 0755 "$LOCAL_BINARY" "$BIN"
  log "installed local binary"
else
  if [[ -z "$VERSION" ]]; then
    VERSION="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null \
      | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1 || true)"
  fi
  ASSET="aios-linux-$ARCH"
  if [[ -n "$VERSION" ]]; then
    URL="https://github.com/$REPO/releases/download/$VERSION/$ASSET"
  else
    URL="https://github.com/$REPO/releases/latest/download/$ASSET"
  fi
  log "downloading $URL"
  curl -fL --retry 3 --connect-timeout 15 -o "$TMP/aios" "$URL" \
    || die "download failed; build locally with 'make build' and use --local-binary"
  if curl -fsSL -o "$TMP/aios.sha256" "${URL}.sha256" 2>/dev/null && [[ -s "$TMP/aios.sha256" ]]; then
    expected="$(awk '{print $1}' "$TMP/aios.sha256")"
    actual="$(sha256sum "$TMP/aios" | awk '{print $1}')"
    [[ "$expected" == "$actual" ]] || die "checksum mismatch"
    log "checksum verified"
  fi
  install -m 0755 "$TMP/aios" "$BIN"
fi
"$BIN" version || die "binary does not run on this host"
install -d -m 0755 "$PREFIX"
log "architecture: $ARCH, kernel $(uname -r)"

if [[ -z "$ADMIN_PASSWORD" ]]; then
  ADMIN_PASSWORD="$(head -c 18 /dev/urandom | base64 | tr -d '=+/' | cut -c1-20)"
  GENERATED_PASSWORD=1
else
  GENERATED_PASSWORD=0
fi
[[ ${#ADMIN_PASSWORD} -ge 8 ]] || die "admin password must be at least 8 characters"
APASSWORD_FILE="$CONF_DIR/admin-password.txt"

if [[ ! -f "$CONF_DIR/config.yaml" ]]; then
  log "writing configuration to $CONF_DIR/config.yaml"
  cat > "$CONF_DIR/config.yaml" <<YAML
app:
  name: AIOS
  env: production
  data_dir: $DATA_DIR
  log_level: info
  log_format: json

server:
  addr: $BIND:$PORT
  public_url: http://$BIND:$PORT
  read_timeout: 15s
  write_timeout: 120s
  idle_timeout: 120s
  max_body_bytes: 2097152
  cors_origins: []
  enable_graphql: true
  rate_limit:
    enabled: true
    requests_per_minute: 240
    burst: 60
    auth_per_minute: 10

database:
  driver: sqlite
  dsn: $DATA_DIR/aios.db
  max_open_conns: 4
  max_idle_conns: 2
  auto_migrate: true
  seed: true

cache:
  driver: memory
  max_entries: 20000
  default_ttl: 5m

queue:
  driver: memory
  workers: 2
  max_attempts: 3
  visibility_timeout: 10m
  poll_interval: 500ms

vector:
  driver: local
  collection: aios_memories
  dim: 384

security:
  issuer: aios
  access_ttl: 30m
  refresh_ttl: 720h
  allowed_clock_skew: 30s
  bcrypt_cost: 10
  auto_generate_keys: true
  bootstrap:
    email: "$ADMIN_EMAIL"
    name: "Administrator"
    password: "$ADMIN_PASSWORD"

llm:
  default_provider: $PROVIDER
  default_model: $MODEL
  embedding_provider: local
  embedding_model: hash-384
  timeout: 180s
  max_retries: 2
  max_tool_iterations: 4
  max_concurrency: 2
  temperature: 0.7
  top_p: 0.9
  max_tokens: 1024

voice:
  enabled: true
  sample_rate: 16000
  stt:
    provider: whisper_http
    base_url: http://127.0.0.1:8000/v1
    model: whisper-1
  tts:
    provider: piper_http
    base_url: http://127.0.0.1:5000
    voice: en_US-lessac-medium
    format: wav

plugins:
  enabled: true
  dir: $DATA_DIR/plugins
  allow_http: true
  allow_wasm: true

memory:
  max_short_turns: 20
  long_term_enabled: true
  semantic_enabled: true
  retrieval_top_k: 5
  max_context_tokens: 3000

observability:
  metrics_enabled: true
  system_stats_interval: 30s

scheduler:
  enabled: true
  poll_interval: 5s
  max_concurrent: 2
  timezone: UTC
  stale_after: 15m

tools:
  internet_enabled: true
  search_provider: duckduckgo
  block_private_ips: true
  max_search_results: 6

rag:
  enabled: true
  max_document_bytes: 2097152

events:
  driver: memory
YAML
  log "wrote configuration"
else
  log "keeping existing $CONF_DIR/config.yaml"
fi
# Always (re)apply ownership/permissions: the service runs as $APP_USER, so a
# config left as root:root 0640 would make the daemon fail to start.
chown root:"$APP_USER" "$CONF_DIR/config.yaml"
chmod 0640 "$CONF_DIR/config.yaml"
printf '%s\n' "$ADMIN_PASSWORD" > "$APASSWORD_FILE"
chown root:root "$APASSWORD_FILE" 2>/dev/null || true
chmod 0600 "$APASSWORD_FILE"
if [[ $USE_SYSTEMD -eq 1 ]]; then
  log "installing systemd service"
  cat > "$SERVICE" <<UNIT
[Unit]
Description=AIOS — personal AI operating system
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$APP_USER
Group=$APP_USER
WorkingDirectory=$DATA_DIR
Environment=AIOS_CONFIG=$CONF_DIR/config.yaml
Environment=AIOS_APP_DATA_DIR=$DATA_DIR
Environment=GOMEMLIMIT=384MiB
Environment=GOGC=60
ExecStart=$BIN server
Restart=on-failure
RestartSec=3
TimeoutStopSec=30
LimitNOFILE=65536
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$DATA_DIR
ProtectKernelTunables=true
ProtectKernelModules=true
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX
LockPersonality=true

[Install]
WantedBy=multi-user.target
UNIT
  systemctl daemon-reload
  systemctl enable aios >/dev/null 2>&1 || true
  # Preflight: the daemon must be able to read its config and data dir.
  if command -v runuser >/dev/null; then
    runuser -u "$APP_USER" -- test -r "$CONF_DIR/config.yaml" \
      || die "service user $APP_USER cannot read $CONF_DIR/config.yaml (fix: chown root:$APP_USER and chmod 0640)"
    runuser -u "$APP_USER" -- test -w "$DATA_DIR" \
      || die "service user $APP_USER cannot write $DATA_DIR (fix: chown -R $APP_USER:$APP_USER $DATA_DIR)"
  fi
  systemctl restart aios
  log "waiting for the service to become healthy"
  healthy=0
  for _ in $(seq 1 40); do
    if curl -fsS --max-time 2 "http://127.0.0.1:$PORT/healthz" >/dev/null 2>&1; then
      healthy=1
      break
    fi
    sleep 1
  done
  if [[ $healthy -ne 1 ]]; then
    warn "service did not report healthy; last logs:"
    journalctl -u aios -n 30 --no-pager || true
    die "installation incomplete — check the logs above"
  fi
fi

# --- firewall (best effort) -------------------------------------------------
if command -v ufw >/dev/null && ufw status 2>/dev/null | grep -q "Status: active"; then
  ufw allow "$PORT"/tcp >/dev/null 2>&1 || true
  log "opened port $PORT/tcp in ufw"
elif command -v firewall-cmd >/dev/null && firewall-cmd --state >/dev/null 2>&1; then
  firewall-cmd --permanent --add-port="$PORT"/tcp >/dev/null 2>&1 || true
  firewall-cmd --reload >/dev/null 2>&1 || true
  log "opened port $PORT/tcp in firewalld"
fi

PUBLIC_IP="$(curl -fsS --max-time 5 https://api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}')"
echo
log "AIOS installed successfully"
echo "  URL          : http://${PUBLIC_IP}:${PORT}"
echo "  Dashboard    : http://${PUBLIC_IP}:${PORT}/   (login page)"
echo "  Email        : $ADMIN_EMAIL"
if [[ $GENERATED_PASSWORD -eq 1 ]]; then
  echo "  Password     : $ADMIN_PASSWORD   (generated — change it after login)"
else
  echo "  Password     : (the value passed with --admin-password)"
fi
echo "  Password file: $APASSWORD_FILE (0600)"

if [[ $USE_SYSTEMD -eq 1 ]]; then
  echo "  Service      : systemctl status aios / journalctl -u aios -f"
fi
echo "  Config       : $CONF_DIR/config.yaml"
echo "  Data         : $DATA_DIR (sqlite + keys)"
echo "  API docs     : http://${PUBLIC_IP}:${PORT}/openapi.yaml · /graphql"
echo
echo "Optional nginx reverse proxy (TLS terminated by nginx):"
echo "  location / { proxy_pass http://127.0.0.1:$PORT; proxy_http_version 1.1;"
echo "    proxy_set_header Upgrade \$http_upgrade; proxy_set_header Connection \"upgrade\";"
echo "    proxy_set_header Host \$host; proxy_buffering off; }"
echo
echo "Enable a real model (fully offline):"
echo "  ollama pull qwen2.5:1.5b && \\"
echo "  sed -i 's/^  default_provider: .*/  default_provider: ollama/' $CONF_DIR/config.yaml && \\"
echo "  sed -i 's/^  default_model: .*/  default_model: qwen2.5:1.5b/' $CONF_DIR/config.yaml && \\"
echo "  systemctl restart aios"