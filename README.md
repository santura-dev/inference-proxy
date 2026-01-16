# LiteLLM Backend

Go-based LiteLLM proxy with intelligent routing for vLLM and SGLang model servers. Complete deployment stack with Kubernetes manifests, Helm charts, and monitoring.

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        LiteLLM Proxy                             │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────┐  │
│  │   Gateway    │  │    Config    │  │       API            │  │
│  │ - Sticky     │  │ - YAML Load  │  │ - /v1/chat/complete  │  │
│  │   Sessions   │  │ - Validation │  │ - /v1/embeddings     │  │
│  │ - Priority   │  │ - Discovery  │  │ - /metrics           │  │
│  │ - Health     │  │              │  │                      │  │
│  └──────────────┘  └──────────────┘  └──────────────────────┘  │
└────────────────────────────┬────────────────────────────────────┘
                             │
        ┌────────────────────┼────────────────────┐
        │                    │                    │
        ▼                    ▼                    ▼
┌───────────────┐  ┌─────────────────┐  ┌─────────────────┐
│     vLLM      │  │     SGLang      │  │   Prometheus    │
│  - Audio      │  │  - LLM (Mistral)│  │   Grafana       │
│  - LLM        │  │  - Embeddings   │  │                 │
│  - Vision     │  │                 │  │                 │
│  - Embeddings │  │                 │  │                 │
└───────────────┘  └─────────────────┘  └─────────────────┘
```

## Model Catalogue (11 Models)

| Category | Model | Provider | Parameters | Context | Server |
|----------|-------|----------|------------|---------|--------|
| **Audio** | `nvidia/parakeet-tdt-0.6b-v3` | NVIDIA | 0.6B | 4K | vLLM |
| **Audio** | `nvidia/parakeet-tdt-0.6b-v2` | NVIDIA | 0.6B | 4K | vLLM |
| **LLM** | `mistralai/mistral-large-3-675b-instruct-2512-nvfp4` | MistralAI | 675B | 256K | SGLang |
| **LLM** | `Qwen/Qwen3-30B-A3B-Instruct-2507` | Qwen | 30B | 262K | vLLM |
| **LLM** | `openai/gpt-oss-120b` | OpenAI | 120B | 131K | SGLang |
| **LLM** | `Qwen/Qwen3-VL-235B-A22B-Thinking` | Qwen | 235B | 262K | vLLM |
| **LLM** | `meta-llama/llama-4-maverick-17b-128e-instruct` | Meta-Llama | 400B | 300K | SGLang |
| **Embed** | `baai/bge-m3` | BAAI | 567M | 8K | SGLang |
| **Embed** | `sentence-transformers/all-MiniLM-L6-v2` | sentence-transformers | 22M | - | vLLM |
| **Embed** | `intfloat/multilingual-e5-large-instruct` | intfloat | 0.6B | - | vLLM |
| **OCR** | `deepseek/deepseek-ocr` | DeepSeek | 3B | 8K | vLLM |

## Project Structure

```
litellm-backend/
├── cmd/
│   └── litellm-proxy/
│       └── main.go              # Entry point
/
│   └── server.go                # HTTP API with chi router
├──├── api internal/
│   ├── config/
│   │   └── config.go            # Config loading & validation
│   ├── gateway/
│   │   └── gateway.go           # Routing logic, sticky sessions
│   └── models/
│       └── models.go            # Type definitions
├── config.yaml                  # Model catalogue (11 models)
├── Dockerfile                   # Multi-stage build
├── Makefile                     # Build, test, deploy targets
├── go.mod / go.sum              # Dependencies
├── deployments/
│   ├── k8s/                     # Kubernetes manifests
│   │   ├── namespace/           # Namespace, ConfigMap, RBAC
│   │   ├── proxy/               # Proxy deployment + HPA + Ingress
│   │   ├── vllm/                # vLLM servers (audio, llm, vision)
│   │   └── sglang/              # SGLang servers (llm, embed)
│   ├── ome/                     # OME Operator CRDs
│   │   ├── 01-base-models.yaml  # BaseModel definitions
│   │   ├── 02-serving-runtimes.yaml  # ServingRuntime definitions
│   │   └── 03-inference-services.yaml  # InferenceService bindings
│   └── helm/                    # Helm charts
│       └── litellm/
│           ├── Chart.yaml
│           ├── values.yaml
│           ├── templates/
│           │   ├── _helpers.tpl
│           │   ├── deployment-proxy.yaml
│           │   ├── service-proxy.yaml
│           │   ├── hpa-proxy.yaml
│           │   ├── ingress.yaml
│           │   ├── servicemonitor.yaml
│           │   ├── networkpolicy.yaml
│           │   ├── configmap.yaml
│           │   └── pdb.yaml
│           └── files/
│               └── config.yaml
├── scripts/
│   ├── deploy.sh                # Full deployment orchestration
│   └── test.sh                  # API testing script
└── README.md                    # This file
```

## Routing Strategies

### 1. Sticky Sessions (Prefix Caching)
For repeated prompts, route to the same model instance:
- Benefits: 70-90% KV cache reuse
- Implementation: `session_id` in request options

### 2. Priority-Based Routing
Models selected by configured priority (lower = higher priority):
```yaml
mistral-large-3-675b: priority 1
qwen3-30b-a3b: priority 2
nvidia-parakeet-tdt-0.6b-v3: priority 3
```

### 3. Capability-Based Routing
Automatic model selection by request type:
- `chat` → LLM models
- `embeddings` → Embedding models
- `vision` → Vision/OCR models
- `audio` → TTS/Speech models

## Quick Start

### Build from Source

```bash
cd /home/sandra/inference/projects/litellm-backend
make build
./bin/litellm-proxy
```

### Docker

```bash
docker build -t ghcr.io/santura-dev/litellm-backend:latest .
docker run -p 4000:4000 ghcr.io/santura-dev/litellm-backend:latest
```

### Kubernetes

```bash
# Using kubectl manifests
make deploy

# Using Helm
helm install litellm ./deployments/helm/litellm/

# Using deployment script
./scripts/deploy.sh
```

## Configuration

### Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `LITELLM_CONFIG` | Path to config.yaml | No (default: config.yaml) |
| `LITELLM_MASTER_KEY` | Master API key | Yes |
| `INTERNAL_API_KEY` | Internal model access | Yes |
| `LITELLM_ADDR` | Listen address | No (default: :4000) |
| `OTEL_SERVICE_NAME` | OpenTelemetry service name | No |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP endpoint | No |

### API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/ready` | GET | Readiness check |
| `/metrics` | GET | Prometheus metrics |
| `/v1/chat/completions` | POST | Chat completion |
| `/v1/embeddings` | POST | Embeddings |
| `/v1/models` | GET | List models |
| `/models` | GET | Model status |
| `/status` | GET | Detailed status |

### Example Request

```bash
curl -X POST http://localhost:4000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $LITELLM_MASTER_KEY" \
  -d '{
    "model": "mistral-large-3-675b",
    "messages": [{"role": "user", "content": "Hello!"}],
    "max_tokens": 100
  }'
```

## Model Server Endpoints

| Server | Service | Port | Models |
|--------|---------|------|--------|
| vLLM Audio | `vllm-audio.litellm.svc` | 8000 | Parakeet TTS |
| vLLM LLM | `vllm-llm.litellm.svc` | 8000 | Qwen3-30B |
| vLLM Vision | `vllm-vision.litellm.svc` | 8000 | Qwen3-VL, DeepSeek OCR |
| vLLM Embed | `vllm-embed.litellm.svc` | 8000 | MiniLM, Multilingual E5 |
| SGLang LLM | `sglang-llm.litellm.svc` | 30000 | Mistral Large, GPT-OSS, Llama 4 |
| SGLang Embed | `sglang-embed.litellm.svc` | 30000 | BGE M3 |

## Monitoring

### Prometheus Metrics

The proxy exposes metrics at `/metrics`:

- `litellm_requests_total{model, status}` - Total requests
- `litellm_request_duration_seconds{model}` - Request duration
- `litellm_active_requests{model}` - Active requests

### ServiceMonitor

Create a Prometheus ServiceMonitor in the `monitoring` namespace:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: litellm-proxy
  namespace: monitoring
spec:
  endpoints:
    - port: metrics
      path: /metrics
  selector:
    matchLabels:
      app.kubernetes.io/name: litellm-backend
```

### Grafana Dashboard

Import the dashboard from `deployments/helm/litellm/dashboards/`.

## Helm Values

```yaml
proxy:
  replicas: 3
  image:
    repository: santura-dev/litellm-backend
    tag: latest
  resources:
    limits:
      cpu: "2"
      memory: "4Gi"
  autoscaling:
    enabled: true
    minReplicas: 3
    maxReplicas: 10
    targetCPUUtilization: 70

monitoring:
  enabled: true
  serviceMonitor:
    enabled: true
    namespace: monitoring

networkPolicy:
  enabled: true
```

## Dependencies

```go
require (
    github.com/go-chi/chi/v5 v5.0.10      # HTTP routing
    github.com/go-chi/cors v1.2.2         # CORS middleware
    github.com/prometheus/client_golang v1.19.0  # Metrics
    gopkg.in/yaml.v3 v3.0.1               # Config parsing
)
```

## License

Part of santura-dev inference stack.
