# Models & Providers

AIOS is provider-agnostic. Providers are managed from **Models** in the web UI and
applied live — no restart and no config-file editing.

## No API key required

| Provider | Base URL | Notes |
|---|---|---|
| Ollama (local) | `http://127.0.0.1:11434` | `ollama serve`, then pull a model in the UI. Best CPU option. |
| LM Studio | `http://127.0.0.1:1234/v1` | Enable the local server in LM Studio. |
| llama.cpp server | `http://127.0.0.1:8080/v1` | `llama-server -m model.gguf`. |
| vLLM | `http://127.0.0.1:8000/v1` | GPU hosts. |
| Pollinations | `https://text.pollinations.ai/openai` | Public, **rate-limited**, no signup. Never send private data. |

## Free tiers (signup key required)

| Provider | Kind | Example model | Key page |
|---|---|---|---|
| Groq | openai | `llama-3.3-70b-versatile` | console.groq.com |
| Google Gemini | gemini | `gemini-2.0-flash` | aistudio.google.com/apikey |
| OpenRouter | openai | `meta-llama/llama-3.3-70b-instruct:free` | openrouter.ai/keys |
| Mistral | openai | `mistral-small-latest` | console.mistral.ai |
| Cerebras | openai | `llama3.1-8b` | cloud.cerebras.ai |
| Hugging Face Router | openai | `meta-llama/Llama-3.2-3B-Instruct` | huggingface.co/settings/tokens |

Paid presets (OpenAI, Anthropic, DeepSeek) are also in the catalog. Free tiers
change without notice; check the provider's current terms.

## How it works

- `PUT /api/v1/admin/providers/{name}` validates the config, **tests nothing** (use
  the Test button), stores the API key with AES-256-GCM (`Secrets`), rebuilds the
  router and atomically swaps it. In-flight requests finish on the old router.
- Keys are never returned by the API; only `••••abcd` hints.
- Overrides live in the database (`config/providers/override`) and are merged over
  `config.yaml` at boot, so GUI changes survive restarts.
- `POST /api/v1/admin/providers/default` changes routing globally. There is no
  per-user model permission in v1; use RBAC for who can change it.
- Ollama management (`/api/v1/admin/ollama/models`, `/pull`) proxies the local
  daemon. Pulls are asynchronous; the UI polls status.
- If the active provider fails, the router uses ordered fallbacks and marks the
  breaker; the chat shows a `⚠ fallback` notice.

## Security notes

- Do not enable the Pollinations preset for confidential data.
- Prefer HTTPS base URLs. Local HTTP endpoints are trusted by design.
- Provider admin endpoints require the `admin` permission; `models:read` only
  lists providers and health.
- Rotate a key by editing the provider and entering a new one; blank keeps the
  stored key.
