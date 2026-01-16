# Prefix Caching Guide

Reuse KV cache for repeated prompts to dramatically reduce latency and cost.

## What is Prefix Caching?

- **KV Cache** = Pre-computed attention states from tokens
- **Prefix** = System prompt + chat history (shared across requests)
- **Prefix Caching** = Store and reuse KV for repeated prefix

## Why It Matters

```
Request 1: "You are a coding expert. Write a function..."
├── System: "You are a coding expert." ──► KV computed (slow)
├── User: "Write a function..." ──────────► KV computed (slow)
└── Response generated

Request 2: "You are a coding expert. Create a class..."
├── System: "You are a coding expert." ──► KV REUSED (fast! ✓)
├── User: "Create a class..." ───────────► KV computed (slow)
└── Response generated
```

**Result:** ~70-90% savings on cached prefix computation.

## How vLLM Implements Prefix Caching

```python
# Enable in vLLM
from vllm import LLM

llm = LLM(
    model="meta-llama/Llama-3.1-70B-Instruct",
    enable_prefix_caching=True,  # 🔑 Key setting
    tensor_parallel_size=4,
)
```

## How It Works Internally

```
1. Tokenize prompt
2. Identify prefix (system message, chat history)
3. Hash prefix tokens → prefix_hash
4. Check KV cache for prefix_hash
5. If cache hit → reuse cached KV blocks
6. If cache miss → compute new KV blocks
7. Store new KV blocks in cache
```

## Sticky Sessions for Prefix Caching

Route same conversation to same replica for cache reuse.

```yaml
router_settings:
  enable_sticky_routing: true
  sticky_routing_ttl: 7200  # 2 hours
```

**Why required:**
- Each replica has independent KV cache
- Same session → same replica → cache hit
- Different replicas = cold cache

## Cache Hit Flow

```
Request with Session: "session-abc"
         │
         ▼
┌─────────────────────┐
│ LiteLLM Router      │
│ Check sticky session│──► Session "session-abc" → Replica "vllm-1"
└─────────────────────┘
         │
         ▼
┌─────────────────────┐
│ Replica "vllm-1"    │
│ Check KV cache      │──► prefix_hash "abc123" → 95% coverage
└─────────────────────┘
         │
         ▼
┌─────────────────────┐
│ PREFILL PHASE       │
│ • Cached blocks: REUSE
│ • New blocks: COMPUTE
└─────────────────────┘
         │
         ▼
Response returned with ~500ms (vs 2000ms without cache)
```

## Comparison: Cached vs Non-Cached

| Metric | No Cache | With Prefix Cache |
|--------|----------|-------------------|
| First request | 2000ms | 2000ms |
| Repeated request | 2000ms | 500ms |
| GPU compute | 100% | 30% |
| Cost | $0.01 | $0.003 |

## Related

- [vLLM Prefix Caching](https://docs.vllm.ai/en/stable/features/automatic_prefix_caching/)
- [Routing Strategies](routing.md)
