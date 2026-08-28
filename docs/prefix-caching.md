# Prefix Caching Guide

Reuse KV cache for repeated prompts to reduce latency. The caching itself happens in vLLM and SGLang, on the backend. inference-proxy's job is to make sure repeat traffic finds the same warm cache.

## What is Prefix Caching?

- **KV cache** = pre-computed attention states from tokens
- **Prefix** = system prompt plus chat history, shared across requests in a conversation
- **Prefix caching** = store and reuse KV for a repeated prefix

```
Request 1: "You are a coding expert. Write a function..."
├── System: "You are a coding expert." ──► KV computed
├── User: "Write a function..." ──────────► KV computed
└── Response generated

Request 2: "You are a coding expert. Create a class..."
├── System: "You are a coding expert." ──► KV REUSED
├── User: "Create a class..." ───────────► KV computed
└── Response generated
```

## Enabling it on the backend

vLLM:

```bash
vllm serve meta-llama/Llama-3.1-8B-Instruct \
  --enable-prefix-caching
```

SGLang:

```bash
python -m sglang.launch_server \
  --model-path Qwen/Qwen2.5-7B-Instruct \
  --enable-prefix-caching
```

The example manifests in `deployments/k8s/` already pass these flags.

## Why sticky sessions matter

Each backend process has its own KV cache. If two requests from the same conversation land on different backends, the second one starts cold. That is the whole reason inference-proxy pins sessions:

- the session id comes from `X-Session-ID` or `options.session_id`
- after the first successful response, the session is pinned to that backend
- later turns reuse the warm prefix on the same backend

Without sticky routing, a load balancer spreads a conversation across replicas and the cache never warms up.

## Cache hit flow

```
Request with session "conversation-42"
              │
              ▼
   ┌──────────────────────────┐
   │ inference-proxy          │
   │ session -> backend A     │
   └──────────────────────────┘
              │
              ▼
   ┌──────────────────────────┐
   │ backend A (vLLM/SGLang)  │
   │ prefix already in cache  │──► prefill reuses cached blocks
   └──────────────────────────┘
              │
              ▼
        fast first token
```

## Related

- [vLLM Prefix Caching](https://docs.vllm.ai/en/stable/features/automatic_prefix_caching/)
- [Routing](routing.md)
