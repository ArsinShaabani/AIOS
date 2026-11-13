# Security

AIOS is designed for hostile networks and untrusted model output. This document
describes the threat model, controls, and operational requirements.

## 1. Threat model

Assumed attackers: network attackers, malicious websites fetched by tools,
prompt-injection content, stolen device tokens, malicious plugins, and a curious
LAN neighbour. Not assumed: physical access to the host, a compromised kernel, or
a malicious administrator (admins can read data by design).

## 2. Authentication

- Passwords: bcrypt (cost 10 default, 8–14 allowed). Minimum 8 characters,
  maximum 256. Login timing is uniform (dummy hash for unknown accounts).
- Access tokens: HS256 JWT, 15 min default, issuer-checked, `typ=access`,
  clock skew 30 s.
- Refresh tokens: 32 bytes of CSPRNG entropy, stored **only as SHA-256 hashes**,
  rotated on every refresh. Presenting an already-rotated token is treated as
  theft and revokes the entire token family.
- Device tokens: `dev_…`, stored as bcrypt hash plus a lookup prefix; shown once.
- API keys: `aios_…`, stored as SHA-256, scoped to a subset of the owner's
  permissions; granting a permission the owner lacks is rejected.
- WebSocket: bearer header, or `?token=` because browsers cannot set headers.

## 3. Authorisation (RBAC)

| Role | Capabilities |
|---|---|
| admin | everything, including users, plugins, settings, audit, prune |
| operator | devices, agents, queue, logs read, chat/voice, memory |
| user | chat, voice, own devices/memories, read agents/models |
| viewer | chat and read-only device/agent data |
| device | device-scoped endpoints only |

Ownership checks are enforced in services (sessions, memories, devices, tasks),
not only in handlers. API key scopes narrow the role; they never widen it.

## 4. Data protection

- Keys: JWT signing key (48 bytes) and AES-256 key are generated on first boot
  into `data/keys/*` with 0600, or supplied via env (`security.jwt_secret`,
  `security.encryption_key` as 32-byte hex/base64). Losing the encryption key
  makes secrets unrecoverable by design.
- At rest: secrets (SMTP password, provider tokens) are AES-256-GCM encrypted in
  `settings`. Password hashes and refresh/device/API tokens are hashed.
- In transit: TLS via the built-in server or a reverse proxy; HSTS should be set
  at the proxy. WebSocket and SSE use the same origin.
- Logs: the central slog handler redacts `password`, `token`, `api_key`,
  `authorization`, `cookie`, provider keys and similar attributes.
- Audit trail: logins, refreshes, user/role changes, device commands, plugin
  installs, memory wipes, settings changes and maintenance. Append-only API,
  90-day default retention on prune.

## 5. Application hardening

- SSRF: web fetch/pages use a dial-time resolver that rejects loopback, private,
  link-local, multicast and unspecified IPs; only http/https; max 5 redirects;
  response size caps.
- Request limits: 1 MB JSON (32 MB audio), strict JSON decode, bounded
  collections (32 capabilities, 32 tools, 12000-character messages).
- Rate limiting: per-IP token bucket (disabled only for `/healthz`, `/readyz`,
  `/metrics`) and a stricter per-IP+email login bucket.
- Security headers: `nosniff`, `DENY` framing, `no-referrer`, restrictive CSP for
  the dashboard, permissions policy.
- CORS: explicit allow-list; wildcard rejected in production.
- Errors: typed codes; internal causes are logged with a request id, never
  returned to clients.
- Device safety: capability values are coerced and bounded (min/max/enum), so a
  model or plugin cannot exceed declared actuator limits.

## 6. Plugin model

Plugins run out of process; AIOS never loads plugin code into its address space.
Admins install manifests, permissions are validated against a known set, HTTP
egress can be restricted with `plugins.allowed_hosts`, and execution is bounded by
time and response size. Treat plugins as you would a browser extension: install
only from sources you trust.

## 7. Production checklist

- [ ] `app.env: production`, exact `server.cors_origins`
- [ ] `security.jwt_secret`/`encryption_key` provided via env or keys persisted
- [ ] bootstrap password set and then rotated; admin account uses a unique email
- [ ] TLS terminated (built-in or proxy); firewall exposes only 80/443
- [ ] Postgres/Redis not exposed publicly; strong passwords
- [ ] `plugins.allowed_hosts` set if plugins are enabled
- [ ] backups for database + `data/keys`; restore tested
- [ ] Prometheus alerts from `deploy/prometheus/alerts.yml` loaded
- [ ] `aios doctor` reports `ok`

## 8. Incident response

1. `POST /api/v1/admin/users/{id}/revoke-tokens` for the affected user.
2. Rotate affected device tokens; disable suspicious plugins.
3. Inspect `GET /api/v1/admin/audit` for the time window and actor.
4. If a key leaked, rotate `security.encryption_key` (re-enter secrets) and the
   JWT secret (forces all sessions to re-authenticate).
5. Export and preserve logs/database before pruning.

## 9. Known limitations

- No built-in MFA yet (roadmap); use a proxy/IdP for SSO if required.
- API keys are hashed, so the plaintext cannot be recovered or displayed later.
- The local hash embedder is lexical, not semantic-grade; enable a real embedding
  provider for higher-quality recall. It never sends data anywhere by itself.
- Plugin signing and a WASM sandbox are roadmap items; today isolation is
  process/HTTP-level.
