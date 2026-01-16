# OME Operator Guide

Declarative model deployment on Kubernetes using OME (Open Model Endpoint).

## What is OME?

- **Kubernetes Operator** - Automates model deployment lifecycle
- **CRDs** - Custom Resource Definitions for model management
- **Reconciliation** - Watches desired state, applies to cluster

## Core CRDs

### BaseModel
Defines model identity and resources.

```yaml
apiVersion: serving.kserve.io/v1alpha1
kind: BaseModel
metadata:
  name: llama-3.1-70b-instruct
spec:
  image: vllm/vllm-openai:latest
  model: meta-llama/Llama-3.1-70B-Instruct
  resources:
    limits:
      nvidia.com/gpu: 4
  storage:
    size: 100Gi
  env:
    - name: HF_TOKEN
      valueFrom:
        secretKeyRef:
          name: hf-secrets
          key: token
```

### ServingRuntime
Defines inference server configuration.

```yaml
apiVersion: serving.kserve.io/v1alpha1
kind: ServingRuntime
metadata:
  name: vllm-openai-runtime
spec:
  supportedModelFormats:
    - name: openai
      version: "1.0.0"
  containers:
    - name: inference-server
      image: vllm/vllm-openai:latest
      command: [vllm, serve]
      args:
        - --enable-prefix-caching
        - --tensor-parallel-size
        - "4"
      ports:
        - containerPort: 8000
      resources:
        limits:
          nvidia.com/gpu: 4
  grpcDataPlane: 2
  grpcEndpoint: "port"
```

### InferenceService
Deploys a model to production.

```yaml
apiVersion: serving.kserve.io/v1alpha1
kind: InferenceService
metadata:
  name: llama-3.1-70b-production
spec:
  baseModel:
    name: llama-3.1-70b-instruct
  runtime: vllm-openai-runtime
  minReplicas: 1
  maxReplicas: 10
  scaleTarget: 50  # Requests per replica before scaling
```

## Controller Workflow

```
User creates InferenceService
           │
           ▼
┌─────────────────────┐
│ OME Controller      │
│ 1. Watch CRD       │
│ 2. Read BaseModel  │
│ 3. Read Runtime    │
│ 4. Generate K8s    │──► Deployment
│ 5. Generate K8s    │──► Service
│ 6. Generate K8s    │──► HPA
│ 7. Apply to Cluster│
│ 8. Wait for ready  │
│ 9. Update status   │
└─────────────────────┘
           │
           ▼
InferenceService ready → Traffic flows to model
```

## Generated Resources

```yaml
# Deployment (created by OME)
apiVersion: apps/v1
kind: Deployment
metadata:
  name: vllm-llama-70b-instruct
spec:
  replicas: 2
  template:
    spec:
      containers:
      - name: vllm
        image: vllm/vllm-openai:latest
        args:
          - --model
          - meta-llama/Llama-3.1-70B-Instruct
          - --enable-prefix-caching
          - --tensor-parallel-size
          - "4"
        resources:
          limits:
            nvidia.com/gpu: 4

---
# Service (created by OME)
apiVersion: v1
kind: Service
metadata:
  name: vllm-llama-70b-instruct
spec:
  selector:
    app: vllm-llama-70b-instruct
  ports:
    - port: 8000
      targetPort: 8000

---
# HPA (created by OME)
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: vllm-llama-70b-instruct-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: vllm-llama-70b-instruct
  minReplicas: 1
  maxReplicas: 10
  metrics:
    - type: Pods
      pods:
        metric:
          name: http_requests_per_second
        target:
          type: AverageValue
          averageValue: "50"
```

## Deployment

```bash
# Install OME operator
kubectl apply -f https://github.com/kserve/kserve/releases/download/v0.12.0/kserve.yaml

# Deploy BaseModels
kubectl apply -f deployments/ome/models/

# Deploy ServingRuntimes
kubectl apply -f deployments/ome/runtimes/

# Deploy InferenceServices
kubectl apply -f deployments/ome/inference-services/

# Check status
kubectl get inferenceservices
```

## Related

- [KServe/OME Docs](https://kserve.github.io/website/)
- [Model Servers](model-servers.md)
