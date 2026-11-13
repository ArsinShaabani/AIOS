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

## 7. Testing

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
