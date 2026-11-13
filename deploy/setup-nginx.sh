#!/usr/bin/env bash
# Put AIOS behind the local nginx and pick a certificate that actually exists.
#
#   sudo bash setup-nginx.sh --domain aios.example.com   # real domain + certbot
#   sudo bash setup-nginx.sh --ip                        # self-signed for the server IP
#   sudo bash setup-nginx.sh --domain aios.example.com --no-certbot   # cert already installed
#   sudo bash setup-nginx.sh --ip --default-server        # answer on https://<ip>/ directly
#
# Safe to re-run. Does not remove other vhosts. The previous AIOS vhost is backed
# up before being replaced, and nothing is reloaded when `nginx -t` fails.
set -euo pipefail

AIOS_PORT=""
DOMAIN=""
MODE=""
USE_CERTBOT=1
DEFAULT_SERVER=0
SITE="/etc/nginx/sites-available/aios"
LINK="/etc/nginx/sites-enabled/aios"
CONF="/etc/aios/config.yaml"

log()  { printf '\033[1;34m[nginx-setup]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[nginx-setup]\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31m[nginx-setup]\033[0m %s\n' "$*" >&2; exit 1; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --domain)     DOMAIN="${2:-}"; MODE=domain; shift 2 ;;
    --ip)         MODE=ip; shift ;;
    --port)       AIOS_PORT="${2:-}"; shift 2 ;;
    --no-certbot) USE_CERTBOT=0; shift ;;
    --default-server) DEFAULT_SERVER=1; shift ;;
    -h|--help)    sed -n '2,12p' "$0"; exit 0 ;;
    *) die "unknown flag: $1" ;;
  esac
done

[[ $EUID -eq 0 ]] || die "run as root"
[[ -n "$MODE" ]] || die "use --domain <host> or --ip"
command -v nginx >/dev/null || die "nginx is not installed (apt install nginx)"
command -v openssl >/dev/null || die "openssl is required"

# --- find the AIOS listener -------------------------------------------------
if [[ -z "$AIOS_PORT" ]]; then
  if [[ -f "$CONF" ]]; then
    AIOS_PORT="$(sed -n 's/^  addr: .*:\([0-9]\+\)$/\1/p' "$CONF" | head -1)"
  fi
  AIOS_PORT="${AIOS_PORT:-8080}"
fi
log "AIOS upstream: 127.0.0.1:$AIOS_PORT"
curl -fsS --max-time 3 "http://127.0.0.1:$AIOS_PORT/healthz" >/dev/null \
  || warn "AIOS did not answer on 127.0.0.1:$AIOS_PORT/healthz — check 'systemctl status aios'"

PUBLIC_IP="$(curl -fsS --max-time 5 https://api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}')"
[[ -n "$PUBLIC_IP" ]] || die "cannot determine the public IP"

# --- choose a certificate that exists on this box ---------------------------
CERT=""
KEY=""

find_existing_cert() {
  # Only return a certificate that actually covers the requested host: using a
  # certificate for another name silently breaks TLS in the browser.
  local want="$1" dir
  if [[ -n "$want" && -f "/etc/letsencrypt/live/$want/fullchain.pem" ]]; then
    CERT="/etc/letsencrypt/live/$want/fullchain.pem"
    KEY="/etc/letsencrypt/live/$want/privkey.pem"
    return 0
  fi
  for dir in /etc/letsencrypt/live/*/; do
    [[ -f "$dir/fullchain.pem" && -f "$dir/privkey.pem" ]] || continue
    if cert_covers "$dir/fullchain.pem" "$want"; then
      CERT="$dir/fullchain.pem"
      KEY="$dir/privkey.pem"
      return 0
    fi
  done
  return 1
}

# cert_covers reports whether the certificate names the host (exact, sans, or
# wildcard). Falls back to a best-effort SAN grep on very old openssl builds.
cert_covers() {
  local cert="$1" host="$2" out
  [[ -f "$cert" ]] || return 1
  out="$(openssl x509 -in "$cert" -noout -checkhost "$host" 2>/dev/null || true)"
  if [[ -n "$out" ]]; then
    grep -q 'does match' <<<"$out" && return 0
    return 1
  fi
  openssl x509 -in "$cert" -noout -text 2>/dev/null \
    | grep -A1 'Subject Alternative Name' | grep -qiE "(DNS:[*a-z0-9.-]*${host//./\\.})" 
}

# list_cert_names prints the hostnames of installed certificates, for errors.
list_cert_names() {
  local dir
  for dir in /etc/letsencrypt/live/*/; do
    [[ -d "$dir" ]] || continue
    basename "$dir"
  done | paste -sd, -
}

SERVER_NAME=""
LISTEN_SSL="listen 443 ssl http2;"
LISTEN_80="listen 80;"
if [[ $DEFAULT_SERVER -eq 1 ]]; then
  LISTEN_SSL="listen 443 ssl http2 default_server;"
  LISTEN_80="listen 80 default_server;"
fi

if [[ "$MODE" == "domain" ]]; then
  [[ -n "$DOMAIN" ]] || die "--domain requires a hostname"
  SERVER_NAME="$DOMAIN"
  if find_existing_cert "$DOMAIN"; then
    log "using existing certificate: $CERT"
  elif [[ $USE_CERTBOT -eq 1 ]]; then
    command -v certbot >/dev/null || die "certbot is not installed (apt install certbot python3-certbot-nginx), or pass --no-certbot"
    log "requesting a certificate for $DOMAIN via certbot (DNS must point here already)"
    certbot certonly --nginx -d "$DOMAIN" --non-interactive --agree-tos \
      --register-unsafely-without-email --keep-until-expiring \
      || die "certbot failed — point an A record for $DOMAIN to $PUBLIC_IP first, then re-run"
    CERT="/etc/letsencrypt/live/$DOMAIN/fullchain.pem"
    KEY="/etc/letsencrypt/live/$DOMAIN/privkey.pem"
    [[ -f "$CERT" ]] || die "certbot reported success but $CERT is missing"
  else
    find_existing_cert "$DOMAIN" || die "no certificate covers $DOMAIN (existing: $(list_cert_names)) and --no-certbot was given"
  fi
  PUBLIC_URL="https://$DOMAIN"
else
  # --ip: self-signed certificate so nginx -t passes and TLS works (browser warning).
  SERVER_NAME="$PUBLIC_IP"
  CERT_DIR="/etc/ssl/aios"
  CERT="$CERT_DIR/aios.crt"
  KEY="$CERT_DIR/aios.key"
  install -d -m 0755 "$CERT_DIR"
  if [[ ! -f "$CERT" || ! -f "$KEY" ]]; then
    log "generating a self-signed certificate for $PUBLIC_IP (valid 825 days)"
    openssl req -x509 -newkey rsa:2048 -sha256 -days 825 -nodes \
      -keyout "$KEY" -out "$CERT" \
      -subj "/CN=$PUBLIC_IP" \
      -addext "subjectAltName=IP:$PUBLIC_IP,DNS:$PUBLIC_IP" >/dev/null 2>&1 \
      || die "openssl failed to create the certificate"
    chmod 0600 "$KEY"
  else
    log "reusing self-signed certificate in $CERT_DIR"
  fi
  PUBLIC_URL="https://$PUBLIC_IP"
fi

log "certificate: $CERT"
log "server_name: $SERVER_NAME"
# --- write the vhost --------------------------------------------------------
backup=""
[[ -f "$SITE" ]] && { backup="$SITE.bak.$(date +%Y%m%d%H%M%S)"; cp "$SITE" "$backup"; log "backed up the previous vhost to $backup"; }

cat > "$SITE" <<NGINX
# Managed by AIOS setup-nginx.sh — safe to edit, re-running regenerates it.
server {
    $LISTEN_80
    listen [::]:80$( [[ $DEFAULT_SERVER -eq 1 ]] && echo " default_server" );
    server_name $SERVER_NAME;
    return 301 https://\$host\$request_uri;
}

server {
    $LISTEN_SSL
    listen [::]:443 ssl http2$( [[ $DEFAULT_SERVER -eq 1 ]] && echo " default_server" );
    server_name $SERVER_NAME;

    ssl_certificate     $CERT;
    ssl_certificate_key $KEY;
    ssl_protocols       TLSv1.2 TLSv1.3;
    ssl_session_cache   shared:aios:10m;
    ssl_session_timeout 1h;

    client_max_body_size 32m;
    client_body_timeout  120s;

    # Security headers (the app sets its own CSP for HTML; these are safe).
    add_header X-Content-Type-Options nosniff always;
    add_header Referrer-Policy no-referrer always;

    location / {
        proxy_pass http://127.0.0.1:$AIOS_PORT;
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection \$connection_upgrade;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
        proxy_connect_timeout 15s;
        proxy_buffering off;       # SSE / streaming chat
        proxy_cache off;
    }

    # Streaming + websocket endpoints must never be buffered.
    location ~ ^/api/v1/(chat|ws/) {
        proxy_pass http://127.0.0.1:$AIOS_PORT;
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection \$connection_upgrade;
        proxy_set_header Host \$host;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header X-Accel-Buffering no;
        proxy_read_timeout 3600s;
        proxy_buffering off;
        chunked_transfer_encoding on;
    }
}
NGINX

# `$connection_upgrade` map: add it once to nginx.conf if it is missing.
if ! grep -q 'map \$http_upgrade \$connection_upgrade' /etc/nginx/nginx.conf; then
  log "adding the websocket upgrade map to /etc/nginx/nginx.conf"
  cp /etc/nginx/nginx.conf "/etc/nginx/nginx.conf.bak.$(date +%Y%m%d%H%M%S)"
  python3 - <<'PY' || die "cannot patch /etc/nginx/nginx.conf (add the map manually)"
from pathlib import Path
p = Path('/etc/nginx/nginx.conf')
s = p.read_text()
block = ('map $http_upgrade $connection_upgrade {\n'
         '    default upgrade;\n'
         "    ''      close;\n"
         '}\n\n')
if 'map $http_upgrade $connection_upgrade' not in s:
    idx = s.find('http {')
    if idx < 0:
        raise SystemExit('no http block')
    s = s[:idx + 6] + '\n' + block + s[idx + 6:]
    p.write_text(s)
PY
fi

# --- enable, validate, reload ----------------------------------------------
ln -sfn "$SITE" "$LINK"

# A default_server may already claim 80/443; that is fine, but warn when it does.
if grep -rqs 'default_server' /etc/nginx/sites-enabled/ 2>/dev/null; then
  log "another vhost declares default_server; AIOS answers on Host: $SERVER_NAME"
fi

if ! nginx -t; then
  die "nginx rejected the configuration — nothing was reloaded (previous vhost intact)"
fi
systemctl reload nginx
log "nginx reloaded"

# --- align AIOS with the public URL ----------------------------------------
if [[ -f "$CONF" ]]; then
  python3 - "$CONF" "$PUBLIC_URL" <<'PY' || warn "could not update $CONF; set public_url/cors_origins manually"
import re, sys
path, url = sys.argv[1], sys.argv[2]
s = open(path).read()
s = re.sub(r'(?m)^  public_url:.*$', f'  public_url: {url}', s, count=1)
s = re.sub(r'(?m)^  cors_origins:.*$', f'  cors_origins: ["{url}"]', s, count=1)
open(path, 'w').write(s)
PY
  systemctl reset-failed aios >/dev/null 2>&1 || true
  systemctl restart aios || warn "aios restart failed; check journalctl -u aios"
  sleep 2
fi

# --- verify end to end -----------------------------------------------------
check() {
  local url="$1" out
  out="$(curl -fsSk --max-time 6 -o /dev/null -w '%{http_code}' "$url" 2>/dev/null || true)"
  if [[ "$out" == "200" ]]; then log "OK   $url"; else warn "FAIL $url (http ${out:-none})"; fi
}
check "http://127.0.0.1$([[ "$SERVER_NAME" == "$PUBLIC_IP" ]] && echo ":$AIOS_PORT" || echo "")/healthz"
check "$PUBLIC_URL/healthz"
check "$PUBLIC_URL/"

echo
log "AIOS is published at $PUBLIC_URL"
if [[ "$MODE" == "ip" ]]; then
  echo "  Note: the certificate is self-signed, so the browser shows a warning once."
  echo "  For a clean certificate use a domain:  sudo bash $0 --domain aios.example.com"
fi
if [[ "$MODE" == "domain" ]]; then
  echo "  Cloudflare users: SSL/TLS mode must be 'Full (strict)' with the proxy ON."
fi
echo "  Login: admin@aios.local  (password in /etc/aios/admin-password.txt)"
