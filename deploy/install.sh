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
#   --upgrade               replace the binary, keep config/data, restart service
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
REPAIR=0
PURGE=0
UPGRADE=0

log()  { printf '\033[1;34m[aios]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[aios]\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31m[aios]\033[0m %s\n' "$*" >&2; exit 1; }
# update_bootstrap_credentials rewrites security.bootstrap.email/password inside
# an existing config without touching the rest of the file (and without requiring
# PyYAML). Used when the installer runs again with new credentials.
update_bootstrap_credentials() {
  local file="$1" email="$2" pass="$3" tmp
  [[ -f "$file" ]] || return 1
  tmp="$(mktemp)"
  # Walk the YAML with a tiny state machine: locate `security:`, then its
  # `bootstrap:` child, then replace the `email:`/`password:` keys at that depth.
  awk -v email="$email" -v pass="$pass" '
    function indent(s,   i) { i = 0; while (substr(s, i+1, 1) == " ") i++; return i }
    {
      line = $0
      match(line, /^[ ]*/); ind = RLENGTH
      trimmed = substr(line, ind + 1)
      if (trimmed ~ /^[A-Za-z_]+:/) {
        key = trimmed; sub(/:.*/, "", key)
        if (ind == 0) { in_sec = (key == "security"); in_boot = 0 }
        else if (in_sec && ind == 2) { in_boot = (key == "bootstrap"); boot_ind = ind }
      }
      if (in_sec && in_boot && ind > boot_ind) {
        if (trimmed ~ /^email:/) {
          if (email != "") {
            printf "%*semail: \"%s\"\n", ind, "", email
          } else {
            print line
          }
          next
        }
        if (trimmed ~ /^password:/) {
          if (pass != "") {
            printf "%*spassword: \"%s\"\n", ind, "", pass
          } else {
            print line
          }
          next
        }
      }
      print line
    }
  ' "$file" > "$tmp" || { rm -f "$tmp"; return 1; }
  if [[ -n "$pass" ]]; then
    grep -q "^    password: \"$pass\"" "$tmp" || { rm -f "$tmp"; return 1; }
  fi
  cat "$tmp" > "$file"
  rm -f "$tmp"
  return 0
}

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
    --repair)         REPAIR=1; shift ;;
    --upgrade)        UPGRADE=1; shift ;;
    -h|--help)        sed -n '2,20p' "$0"; exit 0 ;;
    *) die "unknown flag: $1" ;;
  esac
done
[[ $EUID -eq 0 ]] || die "run as root (sudo)"

# --- repair mode: fix permissions on an existing install and restart ---------
if [[ $REPAIR -eq 1 ]]; then
  id -u "$APP_USER" >/dev/null 2>&1 || die "service user $APP_USER does not exist; run the installer normally"
  APP_GROUP="$(id -gn "$APP_USER")"
  log "repairing ownership for user $APP_USER (group $APP_GROUP)"
  install -d -m 0750 -o "$APP_USER" -g "$APP_GROUP" "$DATA_DIR"
  install -d -m 0750 -o root -g "$APP_GROUP" "$CONF_DIR"
  [[ -f "$CONF_DIR/config.yaml" ]] || die "$CONF_DIR/config.yaml is missing; run the installer normally"
  chown root:"$APP_GROUP" "$CONF_DIR/config.yaml"
  chmod 0640 "$CONF_DIR/config.yaml"
  [[ -f "$DATA_DIR/aios.db" ]] && { chown "$APP_USER":"$APP_GROUP" "$DATA_DIR/aios.db"* || true; }
  if command -v runuser >/dev/null; then
    runuser -u "$APP_USER" -- test -r "$CONF_DIR/config.yaml" || die "still unreadable for $APP_USER"
    runuser -u "$APP_USER" -- test -w "$DATA_DIR" || die "data dir not writable for $APP_USER"
  fi
  # Optionally rotate the bootstrap credentials while repairing.
  if [[ -n "$ADMIN_PASSWORD" ]]; then
    if [[ ${#ADMIN_PASSWORD} -lt 12 ]]; then
      die "--admin-password must be at least 12 characters in production mode"
    fi
    update_bootstrap_credentials "$CONF_DIR/config.yaml" "$ADMIN_EMAIL" "$ADMIN_PASSWORD" \
      || die "cannot update credentials in $CONF_DIR/config.yaml"
    printf '%s\n' "$ADMIN_PASSWORD" > "$CONF_DIR/admin-password.txt"
    chmod 0600 "$CONF_DIR/admin-password.txt"
    log "bootstrap credentials updated"
  fi
  # Surface a precise reason before restarting (e.g. short password, bad CORS).
  if [[ -x "$BIN" ]] && ! runuser -u "$APP_USER" -- "$BIN" doctor -config "$CONF_DIR/config.yaml" >/tmp/aios-doctor.out 2>&1; then
    warn "config check failed:"
    tail -12 /tmp/aios-doctor.out
    echo >&2
    echo "Hint: 'production requires security.bootstrap.password with >=12 chars'" >&2
    echo "      means the stored password is too short. Re-run with" >&2
    echo "      --admin-password '<12+ characters>' or set a new one in $CONF_DIR/config.yaml." >&2
    die "fix the issues above, then run --repair again"
  fi
  log "permissions repaired; restarting service"
  systemctl reset-failed aios >/dev/null 2>&1 || true
  systemctl restart aios
  sleep 3
  if systemctl is-active --quiet aios; then
    log "aios is running"
    PORT="$(sed -n 's/^  addr: .*:\([0-9]\+\)$/\1/p' "$CONF_DIR/config.yaml" | head -1)"
    PORT="${PORT:-8080}"
    curl -fsS "http://127.0.0.1:$PORT/healthz" || true
    echo
    log "open http://$(curl -fsS --max-time 5 https://api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}'):$PORT/"
    exit 0
  fi
  journalctl -u aios -n 20 --no-pager || true
  die "service still failing — see logs above"
fi

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
  armv7l|armv7|armv8l) ARCH=armv7 ;;
  armv6l) die "32-bit ARMv6 is not supported by the release build; use --local-binary with 'make build' or a 64-bit OS" ;;
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
# Resolve the actual primary group: it is not guaranteed to equal the username.
APP_GROUP="$(id -gn "$APP_USER")"
log "service user: $APP_USER, primary group: $APP_GROUP"
install -d -m 0750 -o "$APP_USER" -g "$APP_GROUP" "$DATA_DIR"
# Config is owned by root but readable by the service user's *primary group*.
# The group name is resolved from the user, because `useradd --system` may pick a
# different group name than the username on some distributions.
install -d -m 0750 -o root -g "$APP_GROUP" "$CONF_DIR"
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
    [[ -n "$VERSION" ]] || die "cannot detect the latest release; pass --version vX.Y.Z"
  fi
  VERSION_NUM="${VERSION#v}"
  # Release assets are versioned (aios_1.2.0_linux_amd64). Older releases used
  # aios-linux-<arch>; accept both so upgrades keep working.
  CANDIDATES=(
    "aios_${VERSION_NUM}_linux_${ARCH}"
    "aios-linux-${ARCH}"
    "aios_${ARCH}"
  )
  URL=""
  ASSET=""
  for candidate in "${CANDIDATES[@]}"; do
    try_url="https://github.com/$REPO/releases/download/$VERSION/$candidate"
    log "trying $try_url"
    if curl -fL --retry 2 --connect-timeout 15 -o "$TMP/aios" "$try_url" 2>/dev/null; then
      URL="$try_url"; ASSET="$candidate"; break
    fi
  done
  [[ -n "$URL" ]] || die "download failed for $VERSION ($ARCH); check that the release has a linux-$ARCH asset"
  log "downloaded $ASSET"
  expected=""
  for sum in "SHA256SUMS-${VERSION_NUM}.txt" "SHA256SUMS" "${ASSET}.sha256"; do
    if curl -fsSL -o "$TMP/sums" "https://github.com/$REPO/releases/download/$VERSION/$sum" 2>/dev/null && [[ -s "$TMP/sums" ]]; then
      expected="$(awk -v a="$ASSET" '$2 == a || $2 == "*" a {print $1; exit}' "$TMP/sums")"
      [[ -n "$expected" ]] && break
    fi
  done
  if [[ -n "$expected" ]]; then
    actual="$(sha256sum "$TMP/aios" | awk '{print $1}')"
    [[ "$expected" == "$actual" ]] || die "checksum mismatch (expected $expected, got $actual)"
    log "checksum verified"
  else
    warn "no checksum published for $ASSET; skipping verification"
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
# Production mode enforces >= 12 characters on the bootstrap password, so reject
# short secrets up front instead of leaving the service in a crash loop.
if [[ ${#ADMIN_PASSWORD} -lt 12 ]]; then
  die "admin password must be at least 12 characters (got ${#ADMIN_PASSWORD}); production mode rejects shorter secrets. Example: '${ADMIN_PASSWORD}!x'"
fi
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
  max_body_bytes: 8388608
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
  max_context_tokens: 8192

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
elif [[ $UPGRADE -eq 1 ]]; then
  log "upgrade: keeping $CONF_DIR/config.yaml unchanged"
elif [[ -z "$ADMIN_PASSWORD" ]]; then
  log "config exists and no --admin-password given: keeping credentials unchanged"
else
  log "updating credentials in existing $CONF_DIR/config.yaml"
  update_bootstrap_credentials "$CONF_DIR/config.yaml" "$ADMIN_EMAIL" "$ADMIN_PASSWORD" \
    || die "cannot update $CONF_DIR/config.yaml; run with --repair or edit it manually"
fi
# Always (re)apply ownership/permissions: the service runs as $APP_USER, so a
# config left as root:root 0640 would make the daemon fail to start.
chown root:"$APP_GROUP" "$CONF_DIR/config.yaml"
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