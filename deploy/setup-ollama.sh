#!/usr/bin/env bash
# Install a local LLM (Ollama) and point AIOS at it.
#
#   sudo bash setup-ollama.sh                  # auto-pick a model that fits RAM
#   sudo bash setup-ollama.sh --model qwen2.5:3b
#   sudo bash setup-ollama.sh --status          # show what is configured now
#
# Safe to re-run: it only rewrites the `llm:` section of /etc/aios/config.yaml.
set -euo pipefail

MODEL=""
STATUS=0
CONF="/etc/aios/config.yaml"
OLLAMA_URL="http://127.0.0.1:11434"

log()  { printf '\033[1;34m[ollama-setup]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[ollama-setup]\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31m[ollama-setup]\033[0m %s\n' "$*" >&2; exit 1; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --model)   MODEL="${2:-}"; shift 2 ;;
    --url)     OLLAMA_URL="${2:-}"; shift 2 ;;
    --config)  CONF="${2:-}"; shift 2 ;;
    --status)  STATUS=1; shift ;;
    -h|--help) sed -n '2,10p' "$0"; exit 0 ;;
    *) die "unknown flag: $1" ;;
  esac
done

mem_mb() {
  awk '/MemTotal/ {printf "%d", $2/1024}' /proc/meminfo
}

if [[ $STATUS -eq 1 ]]; then
  echo "config: $CONF"
  sed -n '/^llm:/,/^[a-z]/p' "$CONF" 2>/dev/null | head -20 || warn "cannot read $CONF"
  echo "--- ollama ---"
  curl -fsS --max-time 3 "$OLLAMA_URL/api/tags" 2>/dev/null \
    | sed 's/},{/}\n{/g' | sed -n 's/.*"name":"\([^"]*\)".*/installed: \1/p' || echo "not reachable at $OLLAMA_URL"
  echo "--- AIOS ---"
  curl -fsS --max-time 3 http://127.0.0.1:8080/healthz 2>/dev/null || echo "aios not answering"
  exit 0
fi

[[ $EUID -eq 0 ]] || die "run as root"
[[ -f "$CONF" ]] || die "$CONF not found — install AIOS first"

RAM="$(mem_mb)"
log "detected ${RAM} MB RAM"

# --- pick a model that fits -------------------------------------------------
# Speed on a CPU-only box is dominated by parameter count: ~1.5B gives a few
# tokens/s, ~0.5B gives several times that. Default to the fastest tier that
# still follows instructions, and let the operator opt into quality.
pick_model() {
  local ram="$1"
  if   (( ram <= 3000 )); then echo "qwen2.5:0.5b"
  elif (( ram <= 6000 )); then echo "qwen2.5:1.5b"
  elif (( ram <= 10000 )); then echo "qwen2.5:3b"
  else                         echo "qwen2.5:7b"
  fi
}
if [[ -z "$MODEL" ]]; then
  MODEL="$(pick_model "$RAM")"
  log "selected model $MODEL for ${RAM} MB RAM (override with --model)"
fi
# Small context is the single biggest CPU win: attention cost grows with it and
# AIOS only needs a few thousand tokens for personality + retrieved memory.
CTX=2048
(( RAM > 4000 )) && CTX=4096
(( RAM > 10000 )) && CTX=8192
# --- install ollama ---------------------------------------------------------
if ! command -v ollama >/dev/null; then
  log "installing ollama (official installer)"
  curl -fsSL https://ollama.com/install.sh | sh || die "ollama install failed; see https://ollama.com/download"
else
  log "ollama already installed: $(ollama --version 2>/dev/null | head -1)"
fi
systemctl enable --now ollama >/dev/null 2>&1 || true

# --- tune ollama for a CPU-only, single-user host ---------------------------
# These are the settings that actually change perceived speed:
#   KEEP_ALIVE=-1        model stays resident (no 10-30s reload per message)
#   NUM_PARALLEL=1       one generation at a time; parallel requests only thrash
#   MAX_LOADED_MODELS=1  never swap two models in and out of RAM
#   CONTEXT_LENGTH       caps the attention window (biggest CPU cost)
#   KV_CACHE_TYPE=q8_0   halves KV memory so more of the model stays hot
if command -v systemctl >/dev/null && [[ -d /etc/systemd/system ]]; then
  log "tuning ollama for CPU inference (ctx=$CTX, 1 request at a time)"
  install -d -m 0755 /etc/systemd/system/ollama.service.d
  cat > /etc/systemd/system/ollama.service.d/aios-tuning.conf <<EOF
[Service]
Environment="OLLAMA_KEEP_ALIVE=-1"
Environment="OLLAMA_NUM_PARALLEL=1"
Environment="OLLAMA_MAX_LOADED_MODELS=1"
Environment="OLLAMA_CONTEXT_LENGTH=$CTX"
Environment="OLLAMA_FLASH_ATTENTION=1"
Environment="OLLAMA_KV_CACHE_TYPE=q8_0"
EOF
  systemctl daemon-reload || true
  systemctl restart ollama || warn "could not restart ollama; tuning applies after the next restart"
fi

log "waiting for the ollama service"
for _ in $(seq 1 40); do
  curl -fsS --max-time 2 "$OLLAMA_URL/api/tags" >/dev/null 2>&1 && break
  sleep 1
done
curl -fsS --max-time 3 "$OLLAMA_URL/api/tags" >/dev/null 2>&1 \
  || die "ollama is not answering on $OLLAMA_URL (systemctl status ollama)"

# --- pull the model --------------------------------------------------------
if curl -fsS "$OLLAMA_URL/api/tags" | grep -q "\"$MODEL\""; then
  log "model $MODEL is already downloaded"
else
  log "downloading $MODEL (this can take a few minutes)"
  ollama pull "$MODEL" || die "ollama pull $MODEL failed (check disk space / network)"
fi

# --- verify the model actually answers --------------------------------------
log "testing a completion"
resp="$(curl -fsS --max-time 180 "$OLLAMA_URL/api/generate" -H 'Content-Type: application/json' \
  -d "{\"model\":\"$MODEL\",\"prompt\":\"Reply with the single word: ready\",\"stream\":false,\"options\":{\"num_predict\":16}}" 2>/dev/null || true)"
[[ -n "$resp" ]] || die "model $MODEL did not answer; try: ollama run $MODEL"
log "model answered: $(printf '%s' "$resp" | sed -n 's/.*"response":"\([^"]*\)".*/\1/p' | tr -d '\\n' | head -c 80)"

# --- rewrite the llm: section of the AIOS config ---------------------------
# Prefer python3 (stdio only, no PyYAML) and fall back to awk. The section is
# replaced wholesale so a stale `providers:` list can never keep ollama disabled.
write_llm_block() {
  local conf="$1" model="$2" ctx="$3" url="$4" tmp
  tmp="$(mktemp)"
  BLOCK_MODEL="$model" BLOCK_CTX="$ctx" BLOCK_URL="$url" awk '
    BEGIN {
      model = ENVIRON["BLOCK_MODEL"]; ctx = ENVIRON["BLOCK_CTX"]; url = ENVIRON["BLOCK_URL"]
    }
    function emit() {
      print "llm:"
      print "  default_provider: ollama"
      print "  default_model: " model
      print "  embedding_provider: local"
      print "  embedding_model: hash-384"
      print "  timeout: 300s"
      print "  max_retries: 2"
      print "  max_tool_iterations: 4"
      print "  max_concurrency: 1"
      print "  temperature: 0.7"
      print "  top_p: 0.9"
      print "  max_tokens: 1024"
      print "  providers:"
      print "    - name: ollama"
      print "      kind: ollama"
      print "      enabled: true"
      print "      base_url: " url
      print "      default_model: " model
      print "      models:"
      print "        - {name: " model ", context_window: " ctx ", max_output_tokens: 2048}"
      print "    - name: local"
      print "      kind: local"
      print "      enabled: true"
      print "      default_model: local-template"
      print "      embedding_dim: 384"
    }
    /^llm:[[:space:]]*$/ { inside = 1; emit(); next }
    inside && /^[A-Za-z_]/ { inside = 0 }
    !inside { print }
  ' "$conf" > "$tmp" || { rm -f "$tmp"; return 1; }
  grep -q "^  default_provider: ollama" "$tmp" || { rm -f "$tmp"; return 1; }
  cat "$tmp" > "$conf"
  rm -f "$tmp"
  return 0
}

sudo_cp="$CONF.bak.$(date +%Y%m%d%H%M%S)"
cp "$CONF" "$sudo_cp"
log "backed up config to $sudo_cp"

write_llm_block "$CONF" "$MODEL" "$CTX" "$OLLAMA_URL" || die "cannot rewrite the llm section of $CONF"
# Keep ownership/permissions correct for the service user.
group="$(id -gn aios 2>/dev/null || echo aios)"
chown root:"$group" "$CONF" 2>/dev/null || true
chmod 0640 "$CONF"
log "config now uses provider ollama / model $MODEL"

systemctl reset-failed aios >/dev/null 2>&1 || true
systemctl restart aios
sleep 3
systemctl is-active --quiet aios || { journalctl -u aios -n 20 --no-pager; die "aios failed to start"; }
log "aios restarted"

# --- verify through AIOS itself --------------------------------------------
log "verifying AIOS can reach the model"
if command -v aios >/dev/null; then
  if aios doctor -config "$CONF" >/tmp/aios-doctor-ollama.out 2>&1; then
    log "doctor: ok"
  else
    warn "doctor reported problems:"; tail -8 /tmp/aios-doctor-ollama.out
  fi
fi

PORT="$(sed -n 's/^  addr: .*:\([0-9]\+\)$/\1/p' "$CONF" | head -1)"; PORT="${PORT:-8080}"
health="$(curl -fsS --max-time 5 "http://127.0.0.1:$PORT/healthz" 2>/dev/null || true)"
[[ -n "$health" ]] && log "healthz: $health" || warn "aios is not answering on 127.0.0.1:$PORT"

cat <<'DONE'

Next steps in the dashboard:
  1. Open your AIOS URL and sign in.
  2. Settings -> Model Router: the ollama provider should be listed as enabled.
  3. Chat: replies now come from the local model instead of the template.

Useful commands:
  ollama list                 # installed models
  ollama pull qwen2.5:3b      # add a bigger model, then re-run this script with --model
  sudo bash setup-ollama.sh --status
  journalctl -u ollama -f
DONE
