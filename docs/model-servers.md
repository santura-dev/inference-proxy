# Model Servers

vLLM and SGLang setup for inference.

## vLLM

High-throughput LLM inference server with PagedAttention.

### Quick Start

```bash
docker run -d --gpus all \
  -p 8000:8000 \
  -v /data:/data \
  vllm/vllm-openai:latest \
  --model meta-llama/Llama-3.1-70B-Instruct \
  --tensor-parallel-size 4 \
  --enable-prefix-caching \
  --gpu-memory-utilization 0.9
```

### Key Features

- **PagedAttention** - Memory-efficient KV cache management
- **Continuous Batching** - Dynamic batch sizing
- **Prefix Caching** - Reuse KV for repeated prompts
- **Tensor Parallelism** - Multi-GPU inference
- **OpenAI-Compatible API** - `/v1/chat/completions`

### Configuration

```yaml
# vLLM deployment args
--model meta-llama/Llama-3.1-70B-Instruct
--tensor-parallel-size 4          # GPUs per model
--enable-prefix-caching           # KV cache reuse
--gpu-memory-utilization 0.9      # GPU memory fraction
--max-model-len 32768             # Context length
--dtype half                      # Precision (half/float/auto)
--quantization awq                # AWQ/GPTQ for memory savings
```

## SGLang

Alternative inference server with RadixAttention for complex prompts.

### Quick Start

```bash
python -m sglang.launch_server \
  --model-path meta-llama/Llama-3.1-70B-Instruct \
  --host 0.0.0.0 \
  --port 30000 \
  --tp 4 \
  --enable-prefix-caching \
  --mem-fraction-static 0.9
```

### Key Features

- **RadixAttention** - Efficient prefix tree for complex prompts
- **Multi-turn Chat** - Better state management
- **Structured Output** - Constrained decoding
- **Agent Workflows** - Tool use optimization

## Comparison

| Feature | vLLM | SGLang |
|---------|------|--------|
| Caching | PagedAttention | RadixAttention |
| Best For | General production | Complex prompts |
| Multi-turn | Good | Excellent |
| Memory | Optimized | Good |

## When to Use Each

- **vLLM** - Standard chat, high throughput, production workloads
- **SGLang** - Complex reasoning, multi-turn conversations, tool use

## Kubernetes Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: vllm-llama-70b
spec:
  replicas: 2
  template:
    spec:
      containers:
      - name: vllm
        image: vllm/vllm-openai:latest
        ports:
        - containerPort: 8000
        args:
        - --model
        - meta-llama/Llama-3.1-70B-Instruct
        - --tensor-parallel-size
        - "4"
        - --enable-prefix-caching
        resources:
          limits:
            nvidia.com/gpu: 4
```

## Related

- [vLLM Docs](https://docs.vllm.ai/)
- [SGLang Docs](https://docs.sglang.ai/)
- [Prefix Caching](prefix-caching.md)
