# Documentation Index

## Architecture & Concepts

- [Overview](overview.md) - Request lifecycle, routing, health, configuration
- [Architecture](architecture.md) - Stack overview
- [Routing](routing.md) - Routing order, health, failover
- [Prefix Caching](prefix-caching.md) - KV cache reuse and why sticky sessions matter
- [Model Servers](model-servers.md) - vLLM/SGLang setup

## Quick Reference

### Request Flow
```
App -> inference-proxy -> vLLM/SGLang -> App
```

### Key Concepts
- **Sticky Sessions**: Same session ID routes to the same backend
- **Health Tracking**: 3 consecutive 5xx marks a backend unhealthy
- **Prefix Caching**: 70-90% savings on cached prompts on the backend
