# Overview

inference-proxy is a Go reverse proxy that sits in front of vLLM and SGLang model servers. It routes OpenAI-compatible requests to the right backend, keeps multi-turn sessions on the same backend, and fails over when a backend starts erroring.

## Request lifecycle

```
Client
  │  POST /v1/chat/completions
  ▼
inference-proxy
  │  1. authenticate (when a master key is configured)
  │  2. pick a backend (exact model > sticky session > capability + priority)
  │  3. proxy the request, watching the response code
  ▼
vLLM / SGLang backend
  │
  ▼
Client  ◄── response streamed back, flushed per chunk
```

On a 5xx response or connection error, the proxy records a failure and tries the next candidate before any bytes reach the client. Once a backend starts responding, the response is relayed as-is, so streaming works.

## Routing

Candidates are chosen in this order:

1. **Exact model match.** If the request's `model` field names a configured `model_name`, that backend is used first. OpenAI-compatible clients expect the model they asked for.
2. **Sticky session.** If the request carries an `X-Session-ID` header or a `session_id` option, and that session was pinned to a healthy backend, it stays there.
3. **Capability pool.** Otherwise the request is routed to backends matching the inferred capability (chat, embeddings, vision, audio, code-generation), ordered by `priority`. Equal priorities are load balanced randomly.

If the chosen backend fails, the remaining candidates from the same capability pool are tried in order.

## Health and failover

Health is tracked passively, from real traffic:

- every response under 500 marks the backend healthy and resets its error count
- every 5xx response or connection error increments the error count
- after 3 consecutive failures the backend is marked unhealthy and skipped in selection
- if every backend is unhealthy, the full pool is tried anyway, best effort

`GET /status` reports the current state per model.

## Sticky sessions

Each backend has its own KV cache. Prefix caching only pays off when the same conversation keeps hitting the same backend, so the proxy pins sessions:

- the session id comes from the `X-Session-ID` header, or `options.session_id` in the request body
- the pin is recorded after the first successful response
- pins expire after 30 minutes, and the map is bounded to avoid unbounded growth

The pin is a latency optimization, not a correctness guarantee. If the pinned backend is unhealthy, routing falls through to the normal priority order.

## Authentication

When `general_settings.master_key` resolves to a value, every `/v1` request must send `Authorization: Bearer <key>`. The key can be written directly or as an `os.environ.VAR_NAME` reference that the loader substitutes at startup.

`/health`, `/ready`, and `/metrics` are deliberately unauthenticated so Kubernetes probes and Prometheus scraping work without credentials.

## Configuration

```yaml
model_list:
  - model_name: llama-3.1-8b
    litellm_params:
      model: meta-llama/Llama-3.1-8B-Instruct
      api_base: http://vllm-llm:8000/v1
    model_info:
      mode: chat
      capabilities: ["chat", "tools"]
      priority: 1
      tags: ["llm", "vllm"]

general_settings:
  master_key: os.environ.INFERENCE_PROXY_MASTER_KEY
  stream_timeout: 30
  max_retries: 3
  retry_on_failure: true
```

`api_base` follows the LiteLLM convention of including the `/v1` prefix. The proxy keeps its own `/v1` namespace and joins paths so backends see `/v1/chat/completions`, not `/v1/v1/chat/completions`.

## Endpoints

| endpoint | auth | description |
|---|---|---|
| `POST /v1/chat/completions` | yes | chat completions, streaming supported |
| `POST /v1/completions` | yes | legacy completions |
| `POST /v1/embeddings` | yes | embeddings |
| `GET /v1/models` | yes | configured models |
| `GET /models`, `GET /models/{name}` | no | model listing without the `/v1` prefix |
| `GET /status` | no | backend health |
| `GET /health`, `GET /ready` | no | probes |
| `GET /metrics` | no | Prometheus metrics |

## Environment

| variable | default | description |
|---|---|---|
| `INFERENCE_PROXY_CONFIG` | `config.yaml` | path to the config file |
| `INFERENCE_PROXY_ADDR` | `:4000` | listen address |

## Related

- [Architecture](architecture.md)
- [Routing](routing.md)
- [Prefix caching](prefix-caching.md)
- [Model servers](model-servers.md)
