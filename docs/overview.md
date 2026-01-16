# Complete File-by-File Walkthrough

## Overview

Your LiteLLM Backend project is organized into 5 main categories:

```
litellm-backend/
├── cmd/                           ← Application entry point
├── api/                           ← HTTP API handlers
├── internal/                      ← Core logic (config, gateway, models)
├── config.yaml                    ← Model catalogue
├── deployments/                   ← Kubernetes & Helm manifests
├── scripts/                       ← Automation scripts
├── Dockerfile                     ← Container build
├── Makefile                       ← Developer commands
└── README.md                      ← Documentation
```

---

## Part 1: GO APPLICATION CODE

### 1.1 `cmd/litellm-proxy/main.go` - Entry Point (44 lines)

**Purpose:** The very first code that runs when the application starts.

**What it does:**
```go
func main() {
    // 1. Determine config file path (from env or default)
    configPath := "config.yaml"
    if envPath := os.Getenv("LITELLM_CONFIG"); envPath != "" {
        configPath = envPath
    }

    // 2. Load configuration from file
    loader := config.NewLoader(configPath)
    cfg, err := loader.LoadFromEnv()  // Reads config.yaml, parses YAML

    // 3. Create router with config
    router := gateway.NewRouter(cfg)

    // 4. Log startup info
    log.Printf("LiteLLM Backend initialized with %d models", router.GetModelCount())

    // 5. Create and start HTTP server
    server := api.NewServer(router, cfg)
    server.Start(":4000")  // BLOCKS HERE - runs forever
}
```

**When it runs:** Once at startup, then exits (the HTTP server keeps running).

**Key variables:**
- `LITELLM_CONFIG` - Override config file path
- `LITELLM_ADDR` - Override listen address (default `:4000`)

---

### 1.2 `api/server.go` - HTTP API Server (315 lines)

**Purpose:** Handles all HTTP requests, like a web server.

**Structure:**

```go
// Prometheus metrics (lines 24-51)
var (
    requestsTotal      = prometheus.NewCounterVec(...)  // Count requests
    requestDuration    = prometheus.NewHistogramVec(...) // Track latency
    activeRequests     = prometheus.NewGaugeVec(...)     // Current requests
)

// Server struct (lines 53-58)
type Server struct {
    router  *chi.Mux           // HTTP routing
    gateway *gateway.Router    // Routing logic
    config  *models.LiteLLMConfig
    server  *http.Server       // HTTP server
}

// Setup router with endpoints (lines 70-100)
func (s *Server) setupRouter() {
    s.router = chi.NewRouter()
    
    // CORS - allow cross-origin requests
    s.router.Use(cors.Handler)
    
    // Logging and rate limiting middleware
    s.router.Use(requestLogger)
    s.router.Use(rateLimiter)
    
    // Health endpoints
    s.router.Get("/health", s.healthHandler)      // Is it alive?
    s.router.Get("/ready", s.readyHandler)        // Is it ready?
    s.router.Get("/metrics", ...)                  // Prometheus metrics
    
    // OpenAI-compatible API
    s.router.Route("/v1", func(r chi.Router) {
        r.Post("/chat/completions", s.chatCompletionsHandler)
        r.Post("/embeddings", s.embeddingsHandler)
        r.Post("/completions", s.completionsHandler)
        r.Get("/models", s.modelsHandler)
    })
    
    // Admin endpoints
    s.router.Get("/models", s.modelsHandler)
    s.router.Get("/models/{model_name}", s.modelHandler)
    s.router.Get("/status", s.statusHandler)
}
```

**HTTP Endpoints:**

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/health` | GET | Liveness probe - is the process running? |
| `/ready` | GET | Readiness probe - are model servers healthy? |
| `/metrics` | GET | Prometheus metrics for monitoring |
| `/v1/chat/completions` | POST | Chat with LLM models |
| `/v1/embeddings` | POST | Generate embeddings |
| `/v1/models` | GET | List available models |
| `/models` | GET | Model status and health |
| `/status` | GET | Detailed status of all models |

**Key Handlers:**

**`chatCompletionsHandler` (lines 189-237):**
```go
func (s *Server) chatCompletionsHandler(w http.ResponseWriter, r *http.Request) {
    // 1. Parse JSON request body
    var req models.CompletionRequest
    json.NewDecoder(r.Body).Decode(&req)
    
    // 2. Route request to appropriate model
    decision := s.gateway.RouteRequest(&req)
    
    // 3. Track metrics
    activeRequests.WithLabelValues(decision.ModelName).Inc()
    requestsTotal.WithLabelValues(decision.ModelName, "requested").Inc()
    defer requestDuration.WithLabelValues(decision.ModelName).Observe(...)
    
    // 4. Return response
    json.NewEncoder(w).Encode(response)
}
```

**`readyHandler` (lines 107-132):**
```go
func (s *Server) readyHandler(w http.ResponseWriter, r *http.Request) {
    // Check if models are healthy
    status := s.gateway.GetModelStatus()
    unhealthy := count_unhealthy(status)
    
    if unhealthy > len(status)/2 {
        // More than 50% unhealthy = degraded
        w.WriteHeader(http.StatusServiceUnavailable)
        json.NewEncoder(w).Encode({"status": "degraded"})
    } else {
        // Ready to serve
        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode({"status": "ready"})
    }
}
```

**Server startup (lines 277-302):**
```go
func (s *Server) Start(addr string) error {
    s.server = &http.Server{
        Addr:         addr,
        Handler:      s.router,
        ReadTimeout:  30 * time.Second,
        WriteTimeout: 300 * time.Second,
    }
    
    // Start server in background goroutine
    go s.server.ListenAndServe()
    
    // Wait for shutdown signal
    <-quit  // SIGINT or SIGTERM
    return s.server.Shutdown(ctx)  // Graceful shutdown
}
```

---

### 1.3 `internal/config/config.go` - Configuration Loader (209 lines)

**Purpose:** Reads and validates `config.yaml`.

**Key functions:**

**`Load()` (lines 23-41):**
```go
func (l *Loader) Load() (*models.LiteLLMConfig, error) {
    // 1. Read file from disk
    data, err := os.ReadFile(l.configPath)
    
    // 2. Parse YAML into Go structs
    var config models.LiteLLMConfig
    yaml.Unmarshal(data, &config)
    
    // 3. Validate configuration
    l.validate(&config)
    
    return &config, nil
}
```

**`validate()` (lines 43-65):**
```go
func (l *Loader) validate(config *models.LiteLLMConfig) error {
    // Check model_list is not empty
    if len(config.ModelList) == 0 {
        return fmt.Errorf("model_list cannot be empty")
    }
    
    // Check for duplicate models
    seenModels := make(map[string]bool)
    for _, model := range config.ModelList {
        if model.ModelName == "" {
            return fmt.Errorf("model has empty model_name")
        }
        if seenModels[model.ModelName] {
            return fmt.Errorf("duplicate model: %s", model.ModelName)
        }
        seenModels[model.ModelName] = true
    }
    return nil
}
```

**`substituteEnvVars()` (lines 80-100):**
```go
func (l *Loader) substituteEnvVars(config *models.LiteLLMConfig) {
    // Replace "os.environ.API_KEY" with actual env var value
    for _, model := range config.ModelList {
        if model.LiteLLMParams.APIKey == "os.environ.INTERNAL_API_KEY" {
            model.LiteLLMParams.APIKey = os.Getenv("INTERNAL_API_KEY")
        }
    }
}
```

**`FindModel()` (lines 102-110):**
```go
func (l *Loader) FindModel(config *models.LiteLLMConfig, modelName string) *models.ModelConfig {
    // Linear search through models
    for i := range config.ModelList {
        if config.ModelList[i].ModelName == modelName {
            return &config.ModelList[i]  // Return matching model
        }
    }
    return nil  // Not found
}
```

**`FindModelsByCapability()` (lines 112-127):**
```go
func (l *Loader) FindModelsByCapability(config *models.LiteLLMConfig, capability string) []*models.ModelConfig {
    // Find all models with specific capability
    for _, model := range config.ModelList {
        for _, cap := range model.ModelInfo.Capabilities {
            if cap == capability {
                result = append(result, &model)
            }
        }
    }
    return result
}
```

**`GetModelsByPriority()` (lines 141-179):**
```go
func (l *Loader) GetModelsByPriority(config *models.LiteLLMConfig) []*models.ModelConfig {
    // Sort models by priority (insertion sort)
    for i := 1; i < len(models); i++ {
        // Insert models[i] into sorted position
        // Lower priority number = higher priority
    }
    return sorted_models
}
```

---

### 1.4 `internal/gateway/gateway.go` - Routing Logic (308 lines)

**Purpose:** Decides which model server handles each request.

**Router struct (lines 14-21):**
```go
type Router struct {
    config     *models.LiteLLMConfig   // All models in memory
    health     map[string]*ModelHealth // Health status per model
    stickyKeys map[string]string       // session_id -> model_name
    mu         sync.RWMutex            // Thread safety
    muSticky   sync.RWMutex
}
```

**`RouteRequest()` (lines 50-94) - THE MAIN ROUTING FUNCTION:**
```go
func (r *Router) RouteRequest(req *models.CompletionRequest) *models.RoutingDecision {
    // STRATEGY 1: Check for sticky session (prefix caching)
    if req.Options != nil {
        if sessionID, ok := req.Options["session_id"].(string); ok {
            if modelName := r.getStickyModel(sessionID); modelName != "" {
                decision := r.selectModel(modelName)
                decision.Reason = "sticky_session"
                return decision  // Route to same model for cache reuse
            }
        }
    }
    
    // STRATEGY 2: Find models by capability
    capability := r.extractCapability(req)
    modelsByCap := r.loader.FindModelsByCapability(r.config, capability)
    
    if len(modelsByCap) == 0 {
        // No models with capability, use priority fallback
        return r.selectByPriority()
    }
    
    // STRATEGY 3: Filter healthy models
    var healthyModels []*models.ModelConfig
    for _, m := range modelsByCap {
        if health, ok := r.health[m.ModelName]; ok && health.Healthy {
            healthyModels = append(healthyModels, m)
        } else if !ok {
            // Model not yet checked, assume healthy
            healthyModels = append(healthyModels, m)
        }
    }
    
    if len(healthyModels) == 0 {
        // All unhealthy, fallback to priority
        return r.selectByPriority()
    }
    
    // STRATEGY 4: Select by priority with load balancing
    return r.selectByPriorityFrom(healthyModels)
}
```

**`extractCapability()` (lines 111-134):**
```go
func (r *Router) extractCapability(req *models.CompletionRequest) string {
    // Check for explicit capability in request
    if req.Options != nil {
        if cap, ok := req.Options["capability"].(string); ok {
            return cap
        }
    }
    
    // Infer capability from model name
    modelName := req.Model
    switch {
    case contains(modelName, "code"):
        return "code-generation"
    case contains(modelName, "embed"):
        return "embeddings"
    case contains(modelName, "vision") || contains(modelName, "image"):
        return "vision"
    case contains(modelName, "audio") || contains(modelName, "whisper"):
        return "audio"
    default:
        return "chat"
    }
}
```

**`selectByPriorityFrom()` (lines 142-179):**
```go
func (r *Router) selectByPriorityFrom(modelList []*models.ModelConfig) *models.RoutingDecision {
    // Group models by priority
    priorityGroups := make(map[int][]*models.ModelConfig)
    for _, m := range modelList {
        priority := m.ModelInfo.Priority
        priorityGroups[priority] = append(priorityGroups[priority], m)
    }
    
    // Find lowest priority number (highest priority)
    lowestPriority := min(priorityGroups.keys)
    
    // Load balancing: randomly select from same priority group
    group := priorityGroups[lowestPriority]
    selected := group[rand.Intn(len(group))]
    
    return r.selectModel(selected.ModelName)
}
```

**`selectModel()` (lines 181-195):**
```go
func (r *Router) selectModel(modelName string) *models.RoutingDecision {
    model := r.loader.FindModel(r.config, modelName)
    
    return &models.RoutingDecision{
        ModelName: model.ModelName,
        APIBase:   model.LiteLLMParams.APIBase,  // e.g., "http://sglang-llm:30000/v1"
        Priority:  model.ModelInfo.Priority,
        Tags:      model.ModelInfo.Tags,
        Reason:    "priority",
    }
}
```

**Sticky Sessions (lines 197-216):**
```go
func (r *Router) SetStickySession(sessionID, modelName string) {
    r.muSticky.Lock()
    defer r.muSticky.Unlock()
    r.stickyKeys[sessionID] = modelName  // Remember user is on this model
}

func (r *Router) getStickyModel(sessionID string) string {
    r.muSticky.RLock()
    defer r.muSticky.RUnlock()
    return r.stickyKeys[sessionID]  // Get user's assigned model
}
```

**Health tracking (lines 218-241):**
```go
func (r *Router) UpdateHealth(modelName string, healthy bool, responseTime int64) {
    r.mu.Lock()
    defer r.mu.Unlock()
    
    if health, ok := r.health[modelName]; ok {
        health.Healthy = healthy
        health.ResponseTime = responseTime
        if !healthy {
            health.ErrorCount++
        }
    } else {
        r.health[modelName] = &ModelHealth{
            ModelName:    modelName,
            Healthy:      healthy,
            ResponseTime: responseTime,
        }
    }
}
```

---

### 1.5 `internal/models/models.go` - Data Structures

**Purpose:** Defines the shapes of data used throughout the application.

**Core structures:**

```go
// Top-level configuration
type LiteLLMConfig struct {
    ModelList         []ModelConfig
    GeneralSettings   GeneralSettings
    LitellmSettings   LitellmSettings
}

// A single model entry
type ModelConfig struct {
    ModelName     string        // e.g., "mistral-large-3-675b"
    LiteLLMParams LiteLLMParams // How to connect
    ModelInfo     *ModelInfo    // Metadata
}

// How to connect to the model
type LiteLLMParams struct {
    Model:   string  // e.g., "mistralai/mistral-large-3-675b-instruct"
    APIBase: string  // e.g., "http://sglang-llm:30000/v1"
    APIKey:  string  // API key if needed
}

// Model metadata
type ModelInfo struct {
    Mode         string   // e.g., "chat", "embeddings"
    Description  string
    Owner        string   // e.g., "engineering", "ml-platform"
    Capabilities []string // e.g., ["chat", "text", "images"]
    Priority     int      // Lower = higher priority
    Tags         []string // e.g., ["llm", "mistral"]
}

// Routing decision result
type RoutingDecision struct {
    ModelName string   // Which model to use
    APIBase   string   // Where to send request
    Priority  int
    Tags      []string
    Reason    string   // Why this model was chosen
}

// Request/response types
type CompletionRequest struct {
    Model    string                 // Model name
    Messages []Message              // Chat messages
    Stream   bool                   // Stream response?
    Options  map[string]interface{} // Extra options (session_id, etc.)
}

type CompletionResponse struct {
    ID      string
    Object  string
    Created int64
    Model   string
    Choices []Choice
    Usage   Usage
}
```

---

## Part 2: CONFIGURATION

### 2.1 `config.yaml` - Model Catalogue (152 lines)

**Purpose:** Defines all available models and how to connect to them.

**Structure:**
```yaml
model_list:
  - model_name: nvidia-parakeet-tdt-0_6b-v3        # Proxy's name for model
    litellm_params:
      model: nvidia/parakeet-tdt-0.6b-v3           # Actual model ID
      api_base: http://vllm-audio:8000/v1          # Model server endpoint
    model_info:
      mode: audio_speech
      capabilities: ["tts", "audio"]
      priority: 1                                  # Lower = higher priority
      owner: ml-platform

general_settings:
  master_key: os.environ.LITELLM_MASTER_KEY
  stream_timeout: 30
  max_retries: 3

litellm_settings:
  cache_type: redis
  cache_expiry_seconds: 300
```

**Model entries by category:**

**Audio (TTS):**
| model_name | api_base | priority |
|------------|----------|----------|
| nvidia-parakeet-tdt-0_6b-v3 | vllm-audio:8000 | 1 |
| nvidia-parakeet-tdt-0_6b-v2 | vllm-audio:8000 | 2 |

**LLM:**
| model_name | api_base | priority |
|------------|----------|----------|
| mistral-large-3-675b | sglang-llm:30000 | 1 |
| qwen3-30b-a3b | vllm-llm:8000 | 2 |
| openai-gpt-oss-120b | sglang-llm:30000 | 3 |
| qwen3-vl-235b | vllm-vision:8000 | 1 |
| meta-llama-4-maverick | sglang-llm:30000 | 2 |

**Embeddings:**
| model_name | api_base | priority |
|------------|----------|----------|
| baai-bge-m3 | sglang-embed:30000 | 1 |
| sentence-transformers-minilm-l6 | vllm-embed:8000 | 2 |
| intfloat-multilingual-e5-large | vllm-embed:8000 | 3 |

**OCR/Vision:**
| model_name | api_base | priority |
|------------|----------|----------|
| deepseek-ocr | vllm-vision:8000 | 1 |

---

## Part 3: KUBERNETES DEPLOYMENTS

### 3.1 `deployments/k8s/namespace/00-namespace.yaml`

**Purpose:** Creates the namespace and base resources.

**What it creates:**
```yaml
# Namespace
apiVersion: v1
kind: Namespace
metadata:
  name: litellm

# ConfigMap with embedded config.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: litellm-config
data:
  config.yaml: |
    # Full config.yaml content embedded here

# Secrets for API keys
apiVersion: v1
kind: Secret
metadata:
  name: litellm-secrets
stringData:
  LITELLM_MASTER_KEY: "your-key-here"

# ServiceAccount for the proxy
apiVersion: v1
kind: ServiceAccount
metadata:
  name: litellm

# RBAC - what the proxy can do
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: litellm-role
rules:
  - apiGroups: [""]
    resources: ["configmaps", "secrets", "services"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "watch", "delete"]
```

---

### 3.2 `deployments/k8s/proxy/01-deployment.yaml`

**Purpose:** Deploys the LiteLLM proxy.

**Creates 4 resources:**

**1. Deployment (3 replicas):**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: litellm-proxy
spec:
  replicas: 3  # Run 3 copies for HA
  selector:
    matchLabels:
      app: litellm-proxy
  template:
    spec:
      containers:
        - name: litellm-proxy
          image: ghcr.io/santura-dev/litellm-backend:latest
          ports:
            - containerPort: 4000  # HTTP API
            - containerPort: 4001  # Metrics
          env:
            - name: LITELLM_CONFIG
              value: "/etc/litellm/config.yaml"
            - name: LITELLM_MASTER_KEY
              valueFrom:
                secretKeyRef:
                  name: litellm-secrets
                  key: LITELLM_MASTER_KEY
          resources:
            limits:
              cpu: "2"
              memory: "4Gi"
            requests:
              cpu: "500m"
              memory: "1Gi"
          volumeMounts:
            - name: config
              mountPath: /etc/litellm  # Mount config
      volumes:
        - name: config
          configMap:
            name: litellm-config  # Reference ConfigMap
```

**2. Service (ClusterIP):**
```yaml
apiVersion: v1
kind: Service
metadata:
  name: litellm-proxy
spec:
  type: ClusterIP
  ports:
    - port: 4000
      targetPort: 4000
  selector:
    app: litellm-proxy  # Routes to proxy pods
```

**3. HPA (Auto-scaling):**
```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: litellm-proxy
spec:
  scaleTargetRef:
    kind: Deployment
    name: litellm-proxy
  minReplicas: 3
  maxReplicas: 10
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70  # Scale if CPU > 70%
```

**4. Ingress (External access):**
```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: litellm-proxy
  annotations:
    kubernetes.io/ingress.class: nginx
spec:
  rules:
    - host: litellm-api.santura.dev
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: litellm-proxy
                port:
                  number: 4000
  tls:
    - hosts:
        - litellm-api.santura.dev
      secretName: litellm-tls
```

---

### 3.3 `deployments/k8s/vllm/01-audio.yaml`

**Purpose:** Deploys vLLM server for audio models.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: vllm-audio
spec:
  replicas: 1
  template:
    spec:
      containers:
        - name: vllm
          image: vllm/vllm-openai:v0.6.4
          args:
            - "--model"
            - "nvidia/parakeet-tdt-0.6b-v3"  # The actual model
            - "--host"
            - "0.0.0.0"
            - "--port"
            - "8000"
            - "--max-model-len"
            - "4096"
          resources:
            limits:
              nvidia.com/gpu: 1  # Requires 1 GPU

---
apiVersion: v1
kind: Service
metadata:
  name: vllm-audio
spec:
  selector:
    app: vllm-audio
  ports:
    - port: 8000
```

---

### 3.4 `deployments/k8s/vllm/02-llm.yaml`

**Purpose:** Deploys vLLM server for Qwen3-30B LLM.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: vllm-llm
spec:
  replicas: 1
  template:
    spec:
      containers:
        - name: vllm
          image: vllm/vllm-openai:v0.6.4
          args:
            - "--model"
            - "Qwen/Qwen3-30B-A3B-Instruct-2507"
            - "--host"
            - "0.0.0.0"
            - "--port"
            - "8000"
            - "--max-model-len"
            - "262144"
            - "--dtype"
            - "bfloat16"
            - "--tensor-parallel-size"
            - "2"  # Split across 2 GPUs
          resources:
            limits:
              nvidia.com/gpu: 2  # Requires 2 GPUs
              memory: "128Gi"
```

---

### 3.5 `deployments/k8s/vllm/03-vision.yaml`

**Purpose:** Deploys vLLM server for vision models (Qwen3-VL, DeepSeek OCR).

---

### 3.6 `deployments/k8s/sglang/01-llm.yaml`

**Purpose:** Deploys SGLang server for large LLMs (Mistral Large 3, GPT-OSS, Llama 4).

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: sglang-llm
spec:
  replicas: 1
  template:
    spec:
      containers:
        - name: sglang
          image: lmsysorg/sglang:v0.3.0
          args:
            - "--model-path"
            - "mistralai/mistral-large-3-675b-instruct-2512-nvfp4"
            - "--host"
            - "0.0.0.0"
            - "--port"
            - "30000"
            - "--tensor-parallel"
            - "4"  # Split across 4 GPUs
          resources:
            limits:
              nvidia.com/gpu: 4  # Requires 4 GPUs!
```

---

### 3.7 `deployments/k8s/sglang/02-embed.yaml`

**Purpose:** Deploys SGLang server for BGE M3 embeddings.

---

## Part 4: OME OPERATOR CRDS

### 4.1 `deployments/ome/01-base-models.yaml`

**Purpose:** Defines BaseModel CRDs for OME operator.

```yaml
apiVersion: omeloop.io/v1alpha1
kind: BaseModel
metadata:
  name: mistral-large-3-675b
spec:
  name: mistralai/mistral-large-3-675b-instruct-2512-nvfp4
  parameters:
    count: 675000000000  # 675B parameters
    precision: nvfp4
  contextLength: 262144
  capabilities:
    - chat
    - text
    - images
    - tools
```

---

### 4.2 `deployments/ome/02-serving-runtimes.yaml`

**Purpose:** Defines ServingRuntime CRDs (how to serve models).

```yaml
apiVersion: omeloop.io/v1alpha1
kind: ServingRuntime
metadata:
  name: sglang-runtime
spec:
  name: sglang
  version: 0.3.0
  supportedModels:
    - namePattern: "mistral-large-3-675b"
      endpointPrefix: llm
      port: 30000
  container:
    image: lmsysorg/sglang:v0.3.0
    resources:
      limits:
        nvidia.com/gpu: 4
```

---

### 4.3 `deployments/ome/03-inference-services.yaml`

**Purpose:** Creates InferenceService CRDs that bind BaseModels to ServingRuntimes.

```yaml
apiVersion: omeloop.io/v1alpha1
kind: InferenceService
metadata:
  name: mistral-large-3-675b
spec:
  baseModel: mistral-large-3-675b
  runtime: sglang-runtime
  replicas: 1
  autoscaling:
    minReplicas: 1
    maxReplicas: 2
```

---

## Part 5: HELM CHARTS

### 5.1 `deployments/helm/litellm/Chart.yaml`

**Purpose:** Helm chart metadata.

```yaml
apiVersion: v2
name: litellm-backend
description: LiteLLM Proxy with intelligent routing
version: 0.1.0
appVersion: "0.1.0"
```

---

### 5.2 `deployments/helm/litellm/values.yaml`

**Purpose:** Default configuration values for the Helm chart.

```yaml
proxy:
  enabled: true
  replicas: 3
  image:
    repository: santura-dev/litellm-backend
    tag: latest
  autoscaling:
    enabled: true
    minReplicas: 3
    maxReplicas: 10

monitoring:
  enabled: true
  serviceMonitor:
    enabled: true
    namespace: monitoring
```

---

### 5.3 `deployments/helm/litellm/templates/*.yaml`

**Purpose:** Templated Kubernetes manifests.

| Template | Purpose |
|----------|---------|
| `deployment-proxy.yaml` | Proxy Deployment |
| `service-proxy.yaml` | Proxy Service |
| `hpa-proxy.yaml` | HPA |
| `ingress.yaml` | Ingress |
| `servicemonitor.yaml` | Prometheus ServiceMonitor |
| `networkpolicy.yaml` | NetworkPolicy |
| `configmap.yaml` | ConfigMap |
| `pdb.yaml` | PodDisruptionBudget |

---

## Part 6: SUPPORTING FILES

### 6.1 `Dockerfile`

**Purpose:** Multi-stage build for containerization.

```dockerfile
# Build stage
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /litellm-proxy ./cmd/litellm-proxy/

# Runtime stage
FROM alpine:3.19 AS runtime
RUN apk add --no-cache ca-certificates curl
COPY --from=builder /litellm-proxy /usr/local/bin/
COPY --from=builder /app/config.yaml /etc/litellm/config.yaml
USER 1000
EXPOSE 4000 4001
ENTRYPOINT ["litellm-proxy"]
```

---

### 6.2 `Makefile`

**Purpose:** Developer commands.

```makefile
make build          # Build binary
make test           # Run tests
make docker-build   # Build Docker image
make docker-push    # Push to registry
make deploy         # Deploy to K8s
make status         # Check deployment status
make logs           # View logs
make port-forward   # Port forward
```

---

### 6.3 `scripts/deploy.sh`

**Purpose:** Deployment orchestration script.

```bash
#!/bin/bash
check_prerequisites    # Check kubectl, docker
check_cluster          # Verify cluster connectivity

deploy_namespace       # Create namespace
deploy_vllm            # Deploy vLLM servers
deploy_sglang          # Deploy SGLang servers
deploy_proxy           # Deploy proxy

wait_for_deployments   # Wait for all to be ready
show_status            # Display final status
```

---

### 6.4 `scripts/test.sh`

**Purpose:** API testing script.

```bash
#!/bin/bash
test_health      # Is the proxy alive?
test_ready       # Is it ready?
test_models      # Can we list models?
test_chat        # Can we chat?
test_embeddings  # Can we embed?
```

---

## Part 7: COMPLETE REQUEST FLOW DIAGRAM

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          USER SENDS REQUEST                                  │
│                                                                              │
│  curl -X POST https://litellm-api.santura.dev/v1/chat/completions \        │
│    -H "Content-Type: application/json" \                                    │
│    -d '{"model": "mistral-large-3-675b", "messages": [...]}'               │
│                                                                              │
└───────────────────────────────────┬─────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                    NGINX INGRESS CONTROLLER                                  │
│                                                                              │
│  Listens on port 443 (HTTPS)                                                │
│  Sees: litellm-api.santura.dev                                              │
│  Forwards to: litellm-proxy:4000                                            │
│                                                                              │
│  File: deployments/k8s/proxy/01-deployment.yaml (Ingress)                   │
│                                                                              │
└───────────────────────────────────┬─────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                    LITE LLM PROXY POD                                        │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  api/server.go: chatCompletionsHandler()                            │   │
│  │       │                                                             │   │
│  │       1. Parse JSON body                                            │   │
│  │       2. Call gateway.RouteRequest(&req)                            │   │
│  │       3. Make HTTP POST to model server                             │   │
│  │       4. Return response to user                                    │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  internal/gateway/gateway.go: RouteRequest()                        │   │
│  │       │                                                             │   │
│  │       1. Check sticky session                                       │   │
│  │       2. Extract capability ("chat")                                │   │
│  │       3. Find models with capability                                │   │
│  │       4. Filter healthy models                                      │   │
│  │       5. Select by priority                                         │   │
│  │       6. Return: APIBase = "http://sglang-llm:30000/v1"             │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  internal/config/config.go: FindModel()                             │   │
│  │       │                                                             │   │
│  │       Look up model in config.yaml                                  │   │
│  │       Return: api_base, model_name, etc.                            │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  Compiled from: cmd/, api/, internal/                                       │
│  Docker image: ghcr.io/santura-dev/litellm-backend:latest                   │
│                                                                              │
└───────────────────────────────────┬─────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                    PROXY MAKES HTTP CALL                                     │
│                                                                              │
│  POST http://sglang-llm.litellm.svc:30000/v1/chat/completions               │
│                                                                              │
│  Headers:                                                                    │
│    - Content-Type: application/json                                         │
│    - Authorization: Bearer ...                                               │
│                                                                              │
│  Body:                                                                        │
│    {                                                                          │
│      "model": "mistralai/mistral-large-3-675b-instruct-2512-nvfp4",         │
│      "messages": [{"role": "user", "content": "..."}]                        │
│    }                                                                          │
│                                                                              │
└───────────────────────────────────┬─────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                    KUBERNETES SERVICE (sglang-llm)                           │
│                                                                              │
│  File: deployments/k8s/sglang/01-llm.yaml (Service)                         │
│                                                                              │
│  DNS: sglang-llm.litellm.svc.cluster.local                                  │
│  Routes to: sglang-llm pods                                                  │
│                                                                              │
└───────────────────────────────────┬─────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                    SGLANG POD (OME-deployed)                                 │
│                                                                              │
│  File: deployments/ome/03-inference-services.yaml                           │
│                                                                              │
│  Image: lmsysorg/sglang:v0.3.0                                              │
│  Model: mistral-large-3-675b-instruct-2512-nvfp4                            │
│  GPU Memory: ~XXX GB                                                        │
│                                                                              │
│  Inside the pod:                                                             │
│  1. SGLang software receives HTTP request                                    │
│  2. Loads model into GPU memory                                              │
│  3. Runs inference (GPU computation)                                         │
│  4. Returns: {"id": "...", "choices": [{"message": {...}}]}                 │
│                                                                              │
└───────────────────────────────────┬─────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                    RESPONSE FLOWS BACK                                       │
│                                                                              │
│  SGLang Pod → Service → Proxy → Ingress → User                              │
│                                                                              │
│  Response:                                                                    │
│  {                                                                          │
│    "id": "chatcmpl-abc123",                                                 │
│    "choices": [{                                                            │
│      "message": {"role": "assistant", "content": "Hello! How can I help?"}  │
│    }]                                                                        │
│  }                                                                          │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Summary Table

| Category | Files | Purpose |
|----------|-------|---------|
| **Entry Point** | `cmd/litellm-proxy/main.go` | Bootstraps the application |
| **HTTP API** | `api/server.go` | Handles HTTP requests |
| **Config** | `internal/config/config.go` | Loads config.yaml |
| **Routing** | `internal/gateway/gateway.go` | Routes requests to models |
| **Data** | `internal/models/models.go` | Type definitions |
| **Catalogue** | `config.yaml` | Model configuration |
| **K8s Proxy** | `k8s/proxy/*.yaml` | Deploys proxy |
| **K8s vLLM** | `k8s/vllm/*.yaml` | Deploys vLLM servers |
| **K8s SGLang** | `k8s/sglang/*.yaml` | Deploys SGLang servers |
| **OME CRDs** | `ome/*.yaml` | Model operator CRDs |
| **Helm** | `helm/litellm/*.yaml` | Helm chart templates |
| **Scripts** | `scripts/*.sh` | Automation |
| **Docker** | `Dockerfile` | Container build |
| **Makefile** | `Makefile` | Developer commands |

---

Does this comprehensive walkthrough help clarify how everything works together?