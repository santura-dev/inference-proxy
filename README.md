# inference-proxy

![Go](https://img.shields.io/badge/go-%2300ADD8.svg?style=flat&logo=go&logoColor=white) ![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg) ![Docker](https://img.shields.io/badge/docker-ready-blue?logo=docker) ![Kubernetes](https://img.shields.io/badge/kubernetes-compatible-blue?logo=kubernetes) ![vLLM](https://img.shields.io/badge/vLLM-compatible-green) ![Helm](https://img.shields.io/badge/helm-chart-blue?logo=helm)

Go reverse proxy for vLLM and SGLang. Sticky sessions per request ID, health-aware routing, automatic failover on repeated 5xx. Helm charts, HPA, PDB, ServiceMonitor included.

## The problem

Running inference at scale means model servers go down. Pods restart, GPUs run out of memory, network partitions happen. A round-robin load balancer keeps sending requests to a dead backend until a health check fires, and even then it does not remember which backends were failing. Multi-turn conversations lose their KV cache warmth when they bounce between backends, which is where most latency comes from.

## The idea

Three things: route requests to the right backend based on model capability, keep sessions sticky per request ID so multi-turn conversations hit the same backend and its warm KV cache, and fail over gracefully when a backend starts returning repeated 5xx errors. When the backend recovers, it gets re-added to the pool automatically.

Accepts LiteLLM-style `model_list` configs as a compatibility feature, not as its identity. The routing, health tracking, and failover logic is the point. This is Go, not Python. It focuses on routing intelligence and reliability, not config compatibility. It does not implement every LiteLLM parameter.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     inference-proxy                          │
│  ┌──────────┐  ┌──────────┐  ┌──────────────────────────┐  │
│  │ gateway  │  │ config   │  │          api             │  │
│  │ sticky   │  │ yaml load│  │ /v1/chat/completions     │  │
│  │ sessions │  │ validate │  │ /v1/embeddings           │  │
│  │ priority │  │ env vars │  │ /v1/models               │  │
│  │ health   │  │          │  │ /metrics                 │  │
│  └──────────┘  └──────────┘  └──────────────────────────┘  │
└──────────────────────────┬──────────────────────────────────┘
                           │
        ┌──────────────────┼──────────────────┐
        ▼                  ▼                  ▼
┌──────────────┐  ┌──────────────┐  ┌──────────────┐
│    vLLM      │  │   SGLang     │  │ Prometheus   │
│  port 8000   │  │  port 30000  │  │ Grafana      │
└──────────────┘  └──────────────┘  └──────────────┘
```

The gateway proxies requests to the selected backend and tracks health by watching response codes. After 3 consecutive 5xx responses (or connection errors) a backend is marked unhealthy and removed from selection. Requests fail over to the next candidate. A successful response resets the error count.

Sticky sessions work by request ID. Send an `X-Session-ID` header, or a `session_id` field in the request options. The first successful route pins that session to that backend for 30 minutes, so subsequent turns land on the same warm KV cache. This is where most latency savings come from for multi-turn conversations.

Streaming responses are relayed as-is, flushed token by token, so server-sent events work without buffering.

## Example model catalogue

`config.yaml` ships with a small example catalogue. Point each entry at your own model servers.

| model | capabilities | priority | server |
|---|---|---|---|
| llama-3.1-8b | chat, tools | 1 | vLLM |
| qwen-7b | chat, tools | 2 | SGLang |
| bge-m3 | embeddings | 1 | SGLang |

Routing order: exact `model` match from the request, then sticky session, then the capability pool ordered by priority. Lower priority number wins; backends with the same priority are load balanced randomly.

## API

| endpoint | description |
|---|---|
| `POST /v1/chat/completions` | Proxied chat completions, streaming supported |
| `POST /v1/completions` | Proxied legacy completions |
| `POST /v1/embeddings` | Proxied embeddings |
| `GET /v1/models` | Configured models with capabilities |
| `GET /status` | Live backend health |
| `GET /health`, `GET /ready` | Liveness and readiness for Kubernetes |
| `GET /metrics` | Prometheus metrics |

When a master key resolves in the config, every `/v1` request must send `Authorization: Bearer <key>`. Health, readiness, and metrics stay open for probes and scraping.

## Run

```bash
make build
./bin/inference-proxy
```

Or straight from source:

```bash
go run ./cmd/inference-proxy/
```

The proxy listens on `:4000`. Override with `INFERENCE_PROXY_ADDR`, and the config path with `INFERENCE_PROXY_CONFIG`.

## Configuration

Each entry in `model_list` maps a model name to a backend. The format follows LiteLLM for compatibility:

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
```

- `api_base` includes the `/v1` prefix, as in LiteLLM
- `capabilities` drives routing for requests that do not name an exact model
- `priority` orders candidates, lower is better
- `api_key` and `general_settings.master_key` accept `os.environ.VAR_NAME` references and resolve from the environment

## Structure

```
cmd/inference-proxy/main.go    # entry point
api/server.go                  # HTTP API, proxying, auth, metrics
internal/config/config.go      # config loading, validation, env substitution
internal/gateway/gateway.go    # routing, health, failover, sticky sessions
internal/models/models.go      # type definitions
config.yaml                    # example model catalogue
deployments/helm/              # Helm chart (HPA, PDB, ServiceMonitor, NetworkPolicy)
deployments/k8s/               # raw manifests (namespace, vLLM, SGLang, proxy)
docs/                          # architecture, routing, prefix caching, model servers
scripts/                       # deploy + test scripts
```

## Monitoring

The Helm chart ships a ServiceMonitor, and the `/metrics` endpoint exposes:

- `inference_proxy_requests_total{model,status}`
- `inference_proxy_request_duration_seconds{model}`
- `inference_proxy_active_requests{model}`

## Development

```bash
make test      # go test ./...
make lint      # golangci-lint
make build     # bin/inference-proxy
```

## License

MIT
