package gateway

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/santura-dev/litellm-backend/internal/config"
	"github.com/santura-dev/litellm-backend/internal/models"
)

// Router handles request routing to appropriate models.
type Router struct {
	config     *models.LiteLLMConfig
	loader     *config.Loader
	health     map[string]*ModelHealth
	mu         sync.RWMutex
	stickyKeys map[string]string // session_id -> model_name
	muSticky   sync.RWMutex
}

// ModelHealth tracks health status of individual models.
type ModelHealth struct {
	ModelName    string
	LastCheck    time.Time
	Healthy      bool
	ResponseTime int64
	ErrorCount   int
}

// NewRouter creates a new router with the given configuration.
func NewRouter(cfg *models.LiteLLMConfig) *Router {
	return &Router{
		config:     cfg,
		health:     make(map[string]*ModelHealth),
		stickyKeys: make(map[string]string),
	}
}

// NewRouterFromLoader creates a router by loading configuration.
func NewRouterFromLoader(loader *config.Loader) (*Router, error) {
	cfg, err := loader.LoadFromEnv()
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}
	return NewRouter(cfg), nil
}

// RouteRequest routes a completion request to an appropriate model.
func (r *Router) RouteRequest(req *models.CompletionRequest) *models.RoutingDecision {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check for sticky session first (for prefix caching)
	if req.Options != nil {
		if sessionID, ok := req.Options["session_id"].(string); ok {
			if modelName := r.getStickyModel(sessionID); modelName != "" {
				if decision := r.selectModel(modelName); decision != nil {
					decision.Reason = "sticky_session"
					return decision
				}
			}
		}
	}

	// Find models by capability
	capability := r.extractCapability(req)
	modelsByCap := r.loader.FindModelsByCapability(r.config, capability)

	if len(modelsByCap) == 0 {
		// Fall back to priority-based selection
		return r.selectByPriority()
	}

	// Filter healthy models and select by priority
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
		// All models unhealthy, fall back to priority
		return r.selectByPriority()
	}

	// Select from healthy models by priority
	return r.selectByPriorityFrom(healthyModels)
}

// RouteEmbedding routes an embedding request to an appropriate model.
func (r *Router) RouteEmbedding(req *models.EmbeddingRequest) *models.RoutingDecision {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Find embedding models
	embeddingModels := r.loader.FindModelsByCapability(r.config, "embeddings")

	if len(embeddingModels) == 0 {
		return nil
	}

	return r.selectByPriorityFrom(embeddingModels)
}

// extractCapability determines the required capability from the request.
func (r *Router) extractCapability(req *models.CompletionRequest) string {
	// Check for explicit capability in options
	if req.Options != nil {
		if cap, ok := req.Options["capability"].(string); ok {
			return cap
		}
	}

	// Infer capability from model name or request context
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

// selectByPriority selects a model purely by priority.
func (r *Router) selectByPriority() *models.RoutingDecision {
	modelList := r.loader.GetModelsByPriority(r.config)
	return r.selectByPriorityFrom(modelList)
}

// selectByPriorityFrom selects the highest priority model from a list.
func (r *Router) selectByPriorityFrom(modelList []*models.ModelConfig) *models.RoutingDecision {
	if len(modelList) == 0 {
		return nil
	}

	// Group by priority level
	priorityGroups := make(map[int][]*models.ModelConfig)
	priorities := make([]int, 0, len(modelList))

	for _, m := range modelList {
		priority := 999
		if m.ModelInfo != nil {
			priority = m.ModelInfo.Priority
		}
		priorityGroups[priority] = append(priorityGroups[priority], m)
		if len(priorityGroups[priority]) == 1 {
			priorities = append(priorities, priority)
		}
	}

	// Find lowest priority (highest priority number, lower is better)
	lowestPriority := priorities[0]
	for _, p := range priorities {
		if p < lowestPriority {
			lowestPriority = p
		}
	}

	// Randomly select from the lowest priority group (load balancing)
	group := priorityGroups[lowestPriority]
	if len(group) == 1 {
		return r.selectModel(group[0].ModelName)
	}

	selected := group[rand.Intn(len(group))]
	return r.selectModel(selected.ModelName)
}

// selectModel creates a routing decision for a specific model.
func (r *Router) selectModel(modelName string) *models.RoutingDecision {
	model := r.loader.FindModel(r.config, modelName)
	if model == nil {
		return nil
	}

	return &models.RoutingDecision{
		ModelName: model.ModelName,
		APIBase:   model.LiteLLMParams.APIBase,
		Priority:  999,
		Tags:      model.ModelInfo.Tags,
		Reason:    "priority",
	}
}

// SetStickySession establishes a sticky session for a conversation.
func (r *Router) SetStickySession(sessionID, modelName string) {
	r.muSticky.Lock()
	defer r.muSticky.Unlock()
	r.stickyKeys[sessionID] = modelName
}

// getStickyModel retrieves the model for a sticky session.
func (r *Router) getStickyModel(sessionID string) string {
	r.muSticky.RLock()
	defer r.muSticky.RUnlock()
	return r.stickyKeys[sessionID]
}

// ClearStickySession clears a sticky session.
func (r *Router) ClearStickySession(sessionID string) {
	r.muSticky.Lock()
	defer r.muSticky.Unlock()
	delete(r.stickyKeys, sessionID)
}

// UpdateHealth updates the health status of a model.
func (r *Router) UpdateHealth(modelName string, healthy bool, responseTime int64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if health, ok := r.health[modelName]; ok {
		health.LastCheck = time.Now()
		health.Healthy = healthy
		health.ResponseTime = responseTime
		if !healthy {
			health.ErrorCount++
		} else {
			health.ErrorCount = 0
		}
	} else {
		r.health[modelName] = &ModelHealth{
			ModelName:    modelName,
			LastCheck:    time.Now(),
			Healthy:      healthy,
			ResponseTime: responseTime,
			ErrorCount:   0,
		}
	}
}

// GetModelStatus returns the status of all known models.
func (r *Router) GetModelStatus() map[string]*models.ModelStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	status := make(map[string]*models.ModelStatus)
	for name, health := range r.health {
		status[name] = &models.ModelStatus{
			ModelName:    name,
			Status:       r.getStatusString(health),
			LastCheck:    health.LastCheck,
			ResponseTime: health.ResponseTime,
		}
	}

	// Add models not yet checked
	for _, model := range r.config.ModelList {
		if _, ok := status[model.ModelName]; !ok {
			status[model.ModelName] = &models.ModelStatus{
				ModelName: model.ModelName,
				Status:    "unknown",
			}
		}
	}

	return status
}

// getStatusString returns a human-readable status string.
func (r *Router) getStatusString(health *ModelHealth) string {
	if health.Healthy {
		if health.ResponseTime < 100 {
			return "healthy_fast"
		} else if health.ResponseTime < 500 {
			return "healthy_normal"
		}
		return "healthy_slow"
	}
	return "unhealthy"
}

// GetConfig returns the router's configuration.
func (r *Router) GetConfig() *models.LiteLLMConfig {
	return r.config
}

// GetModelCount returns the number of configured models.
func (r *Router) GetModelCount() int {
	return len(r.config.ModelList)
}

// contains checks if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) && containsSubstring(s, substr)))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
