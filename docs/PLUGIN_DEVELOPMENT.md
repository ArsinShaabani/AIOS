# Plugin Development

AIOS plugins are manifest-declared HTTP tools. They run out of process, never
load into the core, and are discovered by the model through normal function
calling. The model is like VSCode/Obsidian: a small manifest, JSON-Schema
parameters, a stable request/response contract.

## 1. Manifest

```json
{
  "name": "weather",
  "version": "1.0.0",
  "description": "Current weather and forecasts",
  "author": "you",
  "homepage": "https://example.com",
  "endpoint": "http://127.0.0.1:7070",
  "permissions": ["internet"],
  "tools": [
    {
      "name": "weather_now",
      "description": "Current weather for a city",
      "method": "POST",
      "path": "/tools/weather_now",
      "parameters": {
        "type": "object",
        "properties": {"city": {"type": "string"}},
        "required": ["city"]
      }
    }
  ]
}
```

Rules: names are lowercase `[a-z0-9._-]`, one to 32 tools, methods GET/POST,
parameters must be valid JSON Schema, permissions must be from
`chat`, `memory:read`, `memory:write`, `devices:read`, `devices:write`,
`internet`. `admin` is never grantable. `plugins.allowed_hosts` (when non-empty)
restricts the endpoint host.

## 2. HTTP contract

AIOS sends:

```
POST {endpoint}{path}
Content-Type: application/json

{
  "tool": "weather_now",
  "arguments": {"city": "Tehran"},
  "user_id": "usr_…",
  "session_id": "ses_…",
  "agent_id": "agent_search",
  "plugin": "weather",
  "permissions": ["internet"]
}
```

For `GET`, arguments are query parameters instead.

Reply with any of:

```json
{"content": "18°C and clear in Tehran", "data": {"temp_c": 18}}
{"content": "city not found", "is_error": true}
```

A plain-text or non-JSON body is accepted and becomes `content`. Keep replies
under 16 KB where possible; AIOS truncates at 16 KB. Non-2xx responses become
tool errors that the model can recover from.

## 3. Minimal server (Go)

```go
package main

import (
  "encoding/json"
  "net/http"
)

type req struct {
  Arguments map[string]any `json:"arguments"`
}

func main() {
  http.HandleFunc("/tools/weather_now", func(w http.ResponseWriter, r *http.Request) {
    var body req
    _ = json.NewDecoder(r.Body).Decode(&body)
    city, _ := body.Arguments["city"].(string)
    // call your weather API here
    _ = json.NewEncoder(w).Encode(map[string]any{
      "content": "22°C and sunny in " + city,
      "data":    map[string]any{"temp_c": 22},
    })
  })
  _ = http.ListenAndServe("127.0.0.1:7070", nil)
}
```

## 4. Install and verify

Dashboard → Plugins → Install (paste the manifest), or:

```bash
curl -s -X POST http://localhost:8080/api/v1/plugins \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d @manifest.json
```

Then ask the assistant something that matches the tool description; the
`agent_general` agent must allow the tool. Add it to a custom agent via
`POST /api/v1/agents` (`tools: ["weather_now"]`) if you want a dedicated agent.

## 5. Development workflow

- `plugins.dir` is scanned at boot for `*.json` manifests (declarative installs
  for air-gapped images). Existing plugins with the same name are updated in place.
- Add tools without restarting by installing/toggling through the API; the host
  re-registers the tool registry atomically.
- `DELETE /api/v1/plugins/{id}` removes the tools immediately.
- Logs: `plugin call failed`, `plugin tool name already registered`, `cannot load
  plugin` are the useful signals.

## 6. Security expectations

- Never trust `user_id` alone for authorization inside the plugin; validate the
  requested action against your own data.
- Do not log the whole request body (it may contain personal memory).
- Validate and bound every argument; AIOS validates capability bounds but not
  arbitrary plugin arguments.
- Timeouts: keep handlers under 15 s. AIOS enforces `plugins.timeout` (20 s
  default) and caps response bytes.
- If your plugin needs secrets, read them from its own environment/secret store;
  AIOS does not pass credentials to plugins.


## 7. WASM plugins (sandboxed)

AIOS can run plugins as **WASI commands** instead of HTTP services. The sandbox
has no filesystem pre-opens, no environment variables and no network; only the
JSON returned on stdout reaches the host. Memory (default 16 MiB) and wall-clock
time (default 10 s) are bounded, and every invocation runs in a fresh runtime.

### Guest ABI

AIOS writes one JSON request to stdin and reads one JSON response from stdout:

```json
{"tool":"echo","arguments":{"text":"hi"},"user_id":"usr_…","session_id":"ses_…","agent_id":"agent_general","plugin":"wasm-echo"}
```

```json
{"content":"echo: hi","data":{"length":2},"is_error":false}
```

Plain-text stdout is also accepted as `content`. Use the Go SDK
([`sdk/wasmplugin`](../sdk/wasmplugin/wasmplugin.go)):

```go
package main

import "github.com/ArsinShaabani/AIOS/sdk/wasmplugin"

func main() {
    wasmplugin.Serve(func(req wasmplugin.Request) wasmplugin.Response {
        return wasmplugin.Response{Content: "echo: " + wasmplugin.String(req.Arguments, "text", "")}
    })
}
```

Build the guest (Go 1.24+ supports WASI commands):

```bash
GOOS=wasip1 GOARCH=wasm go build -trimpath -o echo.wasm ./examples/plugins/wasm-echo
```

Manifest:

```json
{
  "name": "wasm-echo",
  "version": "1.0.0",
  "kind": "wasm",
  "tools": [{"name": "echo", "description": "Echo text", "parameters": {"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}}],
  "signature": "…base64 Ed25519…",
  "wasm_sha256": "…hex…",
  "wasm_size": 1234567
}
```

Install by adding `wasm_module` (base64 of the `.wasm` file) to the same JSON and
POSTing it to `/api/v1/plugins`. The module is stored under `plugins.dir/wasm/`
with mode 0600, and its digest is re-verified on every boot.

Limits: `plugins.max_wasm_bytes`, `wasm_timeout`, `wasm_memory_pages`,
`max_response_bytes`. The guest cannot read files, open sockets or spawn
processes; capabilities exist only through the returned response.

## 8. Signing and the marketplace format

Third-party plugins can be signed with **Ed25519**. The signature covers the
canonical manifest (name, version, description, author, homepage, kind, sorted
permissions, tools, module digest) and the raw WASM bytes. A signed manifest is a
single JSON document that can be distributed as-is; the WASM file travels next
to it.

```bash
aios plugin-keygen -out ./keys          # 0600 private key + .pub
aios plugin-sign -key ./keys/plugin-signing.key \
  -manifest echo.manifest.json -wasm echo.wasm -out echo.signed.json
```

Configure trust on the server:

```yaml
plugins:
  allow_wasm: true
  require_signature: true          # reject unsigned plugins
  trusted_keys:
    - <base64 public key from plugin-keygen>
```

Install:

```bash
WASM_B64=$(base64 -w0 echo.wasm)
python3 - <<PY
import json
m=json.load(open("echo.signed.json")); m["wasm_module"]="$WASM_B64"
print(json.dumps(m))
PY
# POST the produced JSON to /api/v1/plugins
```

Marketplace rules:

- a package is `manifest.json` + optional `plugin.wasm` + optional README;
- the signed manifest must contain `signature`, `wasm_sha256` and `wasm_size`;
- consumers MUST verify against a trusted key before enabling;
- version bumps require a new signature; changing tools, permissions or bytes
  invalidates the old one;
- the admin UI shows `verified` and the SHA-256 so operators can audit artifacts.

Unsigned HTTP plugins remain supported when `require_signature` is false; signed
HTTP manifests can bind a manifest to a publisher without shipping a module.

## 9. Testing

The e2e suite covers installation and invocation patterns. For local testing:

```bash
go test ./tests/e2e/... -run TestPlugins   # if you add a test
curl -s -X POST localhost:7070/tools/weather_now \
  -H 'Content-Type: application/json' \
  -d '{"arguments":{"city":"Tehran"}}'
```

A plugin that passes the manifest validator, answers within the timeout and
returns `{"content": ...}` is compatible with every AIOS transport (chat, voice,
GraphQL, device bridge).
