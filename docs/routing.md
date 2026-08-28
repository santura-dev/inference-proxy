# Routing

How inference-proxy picks a backend for each request.

## Routing order

### 1. Exact model match

If the request's `model` field matches a configured `model_name`, that backend is used first. OpenAI-compatible clients expect the model they asked for, so an explicit name always wins.

```bash
curl http://localhost:4000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model": "llama-3.1-8b", "messages": [{"role": "user", "content": "hi"}]}'
```

### 2. Sticky session

If the request carries a session id, and that session is pinned to a healthy backend, the request goes there. This keeps multi-turn conversations on the same warm KV cache.

The session id comes from the `X-Session-ID` header:

```bash
curl http://localhost:4000/v1/chat/completions \
  -H "X-Session-ID: conversation-42" \
  -H "Content-Type: application/json" \
  -d '{"model": "llama-3.1-8b", "messages": [{"role": "user", "content": "hi"}]}'
```

or from the request options:

```json
{ "model": "llama-3.1-8b", "options": { "session_id": "conversation-42" } }
```

Pins expire after 30 minutes. They are an optimization: if the pinned backend turns unhealthy, routing falls back to the normal order.

### 3. Capability and priority

Otherwise the request is matched to a capability pool by model name hints (`embed`, `vision`, `audio`, `code`) or an explicit `options.capability`. Within the pool, lower `priority` wins. Backends sharing a priority are load balanced randomly.

```yaml
model_info:
  capabilities: ["chat", "tools"]
  priority: 1
```

If nothing matches the capability, all configured models are considered, still ordered by priority.

## Health and failover

Health is tracked from real traffic, so no active probing is required:

1. A response under 500 marks the backend healthy and resets its error count
2. A 5xx response or connection error increments the error count
3. After 3 consecutive failures the backend is marked unhealthy and skipped
4. If every backend is unhealthy, the proxy tries the full pool anyway

On failure the next candidate is tried before response headers reach the client.

## Observing decisions

Routing decisions show up in two places:

- `GET /status` returns per-model health
- the proxy logs the chosen backend and reason for each request

Prometheus counters label outcomes by model, so `inference_proxy_requests_total{status="upstream_error"}` surfaces backends that are failing over.

## Configuration example

```yaml
model_list:
  - model_name: llama-3.1-8b
    litellm_params:
      model: meta-llama/Llama-3.1-8B-Instruct
      api_base: http://vllm-llm:8000/v1
    model_info:
      capabilities: ["chat", "tools"]
      priority: 1

  - model_name: qwen-7b
    litellm_params:
      model: Qwen/Qwen2.5-7B-Instruct
      api_base: http://sglang-llm:30000/v1
    model_info:
      capabilities: ["chat", "tools"]
      priority: 2
```
