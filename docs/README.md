# Documentation Index

## Architecture & Concepts

- [Overview](architecture.md) - Complete stack overview
- [Routing](routing.md) - Request routing strategies
- [Prefix Caching](prefix-caching.md) - KV cache reuse guide
- [Model Servers](model-servers.md) - vLLM/SGLang setup
- [OME Operator](ome-operator.md) - K8s model deployment

## Quick Reference

### Request Flow
```
App → LiteLLM → vLLM/SGLang → Redis → App
```

### Key Concepts
- **Prefix Caching**: 70-90% savings on cached prompts
- **Sticky Sessions**: Same session → same replica
- **OME Operator**: Declarative model deployment

### Related Documentation
- [LiteLLM Docs](https://docs.litellm.ai/)
- [vLLM Docs](https://docs.vllm.ai/)
- [KServe Docs](https://kserve.github.io/website/)
