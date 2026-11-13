# AIOS User Guide

AIOS is a private AI assistant that runs on your own hardware. It chats, remembers,
controls devices, schedules reminders and can talk out loud. Everything works
offline with a local model; cloud providers are optional.

## 1. First login

Open `http://<host>:8080`. Sign in with the bootstrap administrator:

- email: `admin@aios.local` (or what you configured)
- password: the value of `AIOS_ADMIN_PASSWORD`, or the random password printed
  once at first boot.

Change the password immediately (Users → reset password) and set your timezone
and default personality in the Users/Profile views.

## 2. Connect a model

The default `local` provider is a template engine: it explains how to connect a
real model but does not reason. To get real answers:

**Fully offline (recommended for Pi/mini PC)**

```bash
ollama pull qwen2.5:1.5b
```

Set `llm.default_provider: ollama` and `llm.default_model: qwen2.5:1.5b`, restart.
A 1.5B model fits comfortably in 1–2 GB and answers in a second or two on a Pi 4.

**Cloud**

Add a key and enable a provider in `config.yaml` (or environment):

```yaml
llm:
  default_provider: openai
  default_model: gpt-4o-mini
  providers:
    - name: openai
      kind: openai
      enabled: true
      api_key: ${OPENAI_API_KEY}
```

Restart, then check Model Router in the dashboard for green health.

## 3. Chat

- Type in the composer and press Enter (Shift+Enter for a newline).
- Replies stream token by token; tool calls appear as small purple boxes.
- Use **New** for a new conversation; conversations are listed on the left.
- Pick an agent with `agent_id` in the API or by asking naturally (“search the
  web for…”, “turn on the kitchen light”, “remind me…”).
- The 🎙 **Voice** button starts a live conversation (browser mic). Speak after
  the ready state; say “hey aios …” if a wake word is configured.

## 4. Memory

AIOS extracts durable facts automatically when you say things like “my name is…”,
“I prefer…”, “I live in…”, or “remember that…”. It also understands Persian
phrasings such as «اسم من … است» and «یادت باشه …».

- Memory view lists stored facts with kind and importance.
- Delete single entries or wipe everything (privacy).
- Memories are used to personalise answers and can be searched semantically, even
  with the default CPU embedder.
- Secrets (passwords, keys, tokens) are deliberately **not** auto-remembered.

## 5. Personalities

Presets: Balanced, Companion, Professional, Coach, Minimal, همدم (Farsi). You can
create your own (tone, style, humour, emotional level, response length, custom
instructions) and choose it per conversation.

## 6. Devices

Devices → Register device:

1. name, kind (light/robot/sensor/ESP32/Ros2…), protocol, room;
2. capabilities (name + type + min/max);
3. copy the one-time token into your firmware.

Use **Command** to test a capability and **Telemetry** to see live values.
Firmware guides: [DEVICE_INTEGRATION.md](DEVICE_INTEGRATION.md).

## 7. Reminders and automations

Ask: “Remind me to call mum at 18:00” or “Every morning at 8 summarise my day”.
The Scheduler agent converts this into a durable task; results appear as memories
and (if MQTT is enabled) on `aios/users/<id>/notifications`. Manage them in Task
Queue.

## 8. Plugins

Plugins add tools without recompiling. Install a manifest in Plugins → Install
(see [PLUGIN_DEVELOPMENT.md](PLUGIN_DEVELOPMENT.md)). Each plugin declares
permissions and an HTTP endpoint; AIOS enforces timeouts and size limits.

## 9. Administration

- **Overview:** users, sessions, devices, memories, token usage, cost, host load.
- **Users:** create users, roles (admin/operator/user/viewer), disable, reset
  passwords, revoke sessions.
- **Audit Log:** every security-relevant action with actor and IP.
- **Model Router:** provider health, models, routing rules.
- **Task Queue:** durable tasks and queue health.
- **Settings:** global JSON settings (SMTP, integrations); secrets are encrypted.
- **System:** health checks, CPU/memory, maintenance (prune).

## 10. Documents

**Documents** lets you upload reference material (Markdown, text, HTML, JSON, CSV)
and ask questions with citations. Uploads are chunked, embedded and kept in your
private index; nothing is shared between users.

- Drag a file (Markdown, text, HTML, JSON, CSV, text-based PDF) and optional title, then **Upload**.
- **Search** shows the best passages with document name, chunk number and score.
- **Chunks** opens the exact indexed text for auditing.
- The assistant can call `document_search` automatically when a question matches
  your uploaded material; answers include the passage titles/ids.
- Deleting a document removes its chunks and vectors immediately.
- Limits: 2 MiB per document, 500 chunks, configurable in `rag`.


## 11. API keys and integrations

Profile → API keys → create with a scope subset. Use `Authorization: Bearer
aios_…` for scripts. Device firmware uses device tokens instead. GraphQL is at
`/graphql`; the OpenAPI spec at `/openapi.yaml`.

## 12. Privacy and data

- Local SQLite by default; no telemetry leaves the host.
- Cloud model calls only send the prompt you trigger (plus memory context).
- Delete a conversation, a memory, or all memories at any time.
- Back up `data/aios.db` and `data/keys`; the keys are required to decrypt stored
  secrets.

## 13. Troubleshooting

| Problem | Solution |
|---|---|
| Template reply instead of real answers | enable Ollama/OpenAI and restart |
| Login loop | clear site data; check `/readyz` |
| Voice button does nothing | grant microphone permission; HTTPS or localhost required |
| Device stays offline | token, topic prefix, or broker unreachable |
| Chat is slow | smaller model, lower `max_tokens`, check host load |
| Forgot admin password | `aios hash-password`, update the DB, or recreate the DB in dev |

Need more detail? See [API.md](API.md), [DEPLOYMENT.md](DEPLOYMENT.md),
[SECURITY.md](SECURITY.md), [VOICE_SYSTEM.md](VOICE_SYSTEM.md).
