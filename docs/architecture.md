# Architecture Overview

Go reverse proxy for self-hosted inference on Kubernetes.

## High-Level Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                         Application Layer                        │
│           (Web App, API, Batch Jobs, CLI Tools)                 │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                      inference-proxy                             │
│  • OpenAI-compatible API (/v1/chat/completions, /v1/embeddings) │
│  • Capability + priority routing                                │
│  • Sticky sessions per request ID                               │
│  • Passive health tracking and failover                         │
│  • Bearer auth, Prometheus metrics                              │
└────────────────────────────┬────────────────────────────────────┘
                             │
        ┌────────────────────┼────────────────────┐
        │                    │                    │
        ▼                    ▼                    ▼
┌──────────────┐    ┌──────────────┐    ┌──────────────┐
│   vLLM       │    │   SGLang     │    │  Prometheus  │
│  Port:8000   │    │  Port:30000  │    │   Grafana    │
└──────────────┘    └──────────────┘    └──────────────┘
        │                    │
        └────────────────────┼────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Kubernetes Cluster                            │
│  • Deployments + Services for model servers                      │
│  • HPA for the proxy                                             │
│  • GPU scheduling                                                │
│  • Ingress, network policies, PDB                                │
└─────────────────────────────────────────────────────────────────┘
```

## Component Responsibilities

| Component | Responsibility |
|-----------|---------------|
| **inference-proxy** | API gateway, routing, sticky sessions, health, failover, auth, metrics |
| **vLLM** | High-throughput inference with prefix caching |
| **SGLang** | Complex prompt handling, RadixAttention |
| **Prometheus** | Metrics, alerting, dashboards |

## Request Flow

```
1. App -> POST /v1/chat/completions (inference-proxy)
2. Proxy -> authenticate when a master key is configured
3. Proxy -> pick backend: exact model, sticky session, capability + priority
4. Proxy -> forward request to vLLM/SGLang, watch the response code
5. Backend -> process with prefix caching
6. Proxy -> stream the response back, record health and metrics
```

Failover happens before response headers reach the client. If a backend returns 5xx, the proxy tries the next candidate.

## Key Technologies

- **Go** - https://go.dev/
- **vLLM** - Inference: https://docs.vllm.ai/
- **SGLang** - Inference: https://docs.sglang.ai/
- **Kubernetes** - Orchestration: https://kubernetes.io/
