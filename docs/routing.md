# Routing Strategies

How LiteLLM routes requests to optimal models.

## Routing Strategies

### Least-Busy (Recommended)
Route to replica with lowest current load.

```yaml
router_settings:
  routing_strategy: "least-busy"
```

**How it works:**
1. Query each replica's current request count
2. Select replica with fewest active requests
3. Ideal for balancing load across replicas

### Sticky Sessions (Critical for Prefix Caching)
Route same session to same replica for cache reuse.

```yaml
router_settings:
  enable_sticky_routing: true
  sticky_routing_ttl: 7200  # 2 hours
```

**Why it matters:**
- Prefix caching requires same replica
- Cache hit = 70-90% latency/cost savings
- Essential for chat applications

### Weighted Routing
Route percentage-based to different models.

```yaml
router_settings:
  routing_strategy: "weighted"
```

### Fallback Routing
Try backup models if primary fails.

```yaml
router_settings:
  fallbacks:
    - model_name: llama-70b-instruct
      api_base: http://vllm-llama-70b-2:8000
    - model_name: gpt-4-turbo  # External fallback
```

## Request Flow with Routing

```
Request arrives
     │
     ▼
┌─────────────────┐
│ Validate API Key│
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Check Cache     │◄── Redis semantic cache
└────────┬────────┘
         │ Cache miss
         ▼
┌─────────────────┐
│ Extract Prefix  │──┐
│ Route Decision  │  │
└────────┬────────┘  │
         │           │
         ▼           │
┌─────────────────┐  │
│ Sticky Check    │◄─┘
│ (Session→Replica)│
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Select Optimal  │─── Least-busy
│ Replica         │─── Weighted
└────────┬────────┘
         │
         ▼
Forward to Model Server
```

## Configuration Example

```yaml
router_settings:
  routing_strategy: "least-busy"
  
  enable_sticky_routing: true
  sticky_routing_ttl: 7200
  
  num_retries: 3
  timeout: 300
  cooldown_time: 30
  
  fallbacks:
    - model_name: llama-70b-internal
    - model_name: gpt-4-turbo  # External
```
