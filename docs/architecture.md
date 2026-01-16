# Architecture Overview

Complete inference stack for self-hosted GPU infrastructure.

## High-Level Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                         Application Layer                        │
│           (Web App, API, Batch Jobs, CLI Tools)                 │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                      LiteLLM Proxy Server                        │
│  • Unified OpenAI-compatible API                                │
│  • Authentication, routing, caching, metrics                    │
│  • Per-team budgets, rate limits, fallbacks                     │
└────────────────────────────┬────────────────────────────────────┘
                             │
        ┌────────────────────┼────────────────────┐
        │                    │                    │
        ▼                    ▼                    ▼
┌──────────────┐    ┌──────────────┐    ┌──────────────┐
│   vLLM       │    │   SGLang     │    │   External   │
│  (Primary)   │    │  (Complex)   │    │  (Fallback)  │
│  Port:8000   │    │  Port:30000  │    │  OpenAI/Anthr│
└──────────────┘    └──────────────┘    └──────────────┘
        │                    │                    │
        └────────────────────┼────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Kubernetes Cluster                            │
│  • OME Operator manages model deployments                       │
│  • HPA for autoscaling                                          │
│  • GPU scheduling (A100/H100)                                   │
│  • Services, ingress, network policies                          │
└─────────────────────────────────────────────────────────────────┘
```

## Component Responsibilities

| Component | Responsibility |
|-----------|---------------|
| **LiteLLM** | API gateway, routing, caching, auth, metrics |
| **vLLM** | High-throughput inference with prefix caching |
| **SGLang** | Complex prompt handling, RadixAttention |
| **OME Operator** | Declarative model deployment on K8s |
| **Redis** | Response caching, semantic cache, KV metadata |
| **Prometheus** | Metrics, alerting, dashboards |

## Request Flow

```
1. App → POST /v1/chat/completions (LiteLLM)
2. LiteLLM → Validate API key, check cache
3. LiteLLM → Route to optimal model (least-busy, sticky)
4. vLLM/SGLang → Process with prefix caching
5. Redis → Cache response, record metrics
6. LiteLLM → Return response to app
```

## Key Technologies

- **LiteLLM** - Gateway: https://docs.litellm.ai/
- **vLLM** - Inference: https://docs.vllm.ai/
- **SGLang** - Inference: https://docs.sglang.ai/
- **OME/KServe** - Operator: https://kserve.github.io/website/
