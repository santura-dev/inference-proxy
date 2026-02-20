package gateway

import (
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/santura-dev/inference-proxy/internal/config"
	"github.com/santura-dev/inference-proxy/internal/models"
)

// FailureThreshold is the number of consecutive 5xx responses (or connection
// errors) before a backend is marked unhealthy.
const FailureThreshold = 3

const (
	stickyTTL        = 30 * time.Minute
	stickyMaxEntries = 8192
)

// Router handles request routing to configured model backends.
type Router struct {
	config     *models.LiteLLMConfig
	loader     *config.Loader
	health     map[string]*ModelHealth
	mu         sync.RWMutex
	stickyKeys map[string]stickyEntry // session_id -> pinned backend
	muSticky   sync.RWMutex
}

// ModelHealth tracks health status of individual backends.
type ModelHealth struct {
	ModelName    string
	LastCheck    time.Time
	Healthy      bool
	ResponseTime int64
	ErrorCount   int
}

type stickyEntry struct {
	model   string
	expires time.Time
}

// NewRouter creates a new router with the given configuration.
func NewRouter(cfg *models.LiteLLMConfig) *Router {
	return &Router{
		config:     cfg,
		loader:     config.NewLoader(""),
		health:     make(map[string]*ModelHealth),
		stickyKeys: make(map[string]stickyEntry),
	}
}

// RouteCandidates returns an ordered list of routing decisions for a
// completion request: exact model match first (OpenAI-compatible clients
// expect the model they asked for), then sticky session, then capability
// and priority ordered candidates for failover.
func (r *Router) RouteCandidates(req *models.CompletionRequest, sessionID string) []*models.RoutingDecision {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if dec := r.selectModel(req.Model); dec != nil {
		dec.Reason = "exact_match"
		return append([]*models.RoutingDecision{dec}, r.failoverDecisions(req, dec.ModelName)...)
	}

	// Sticky session: keep multi-turn conversations on the same backend
	// for KV cache warmth.
	if sessionID != "" {
		if name := r.getStickyModel(sessionID); name != "" && r.isHealthy(name) {
			if dec := r.selectModel(name); dec != nil {
				dec.Reason = "sticky_session"
				return []*models.RoutingDecision{dec}
			}
		}
	}

	return r.orderByPriority(r.healthyPool(r.capabilityPool(req)))
}

// RouteEmbeddingCandidates returns ordered candidates for an embedding request.
func (r *Router) RouteEmbeddingCandidates(req *models.EmbeddingRequest) []*models.RoutingDecision {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if dec := r.selectModel(req.Model); dec != nil {
		dec.Reason = "exact_match"
		return []*models.RoutingDecision{dec}
	}

	return r.orderByPriority(r.healthyPool(r.loader.FindModelsByCapability(r.config, "embeddings")))
}

// failoverDecisions builds backup candidates from the same capability pool,
// excluding the primary pick.
func (r *Router) failoverDecisions(req *models.CompletionRequest, exclude string) []*models.RoutingDecision {
	var rest []*models.ModelConfig
	for _, m := range r.healthyPool(r.capabilityPool(req)) {
		if m.ModelName != exclude {
			rest = append(rest, m)
		}
	}
	return r.orderByPriority(rest)
}

// capabilityPool returns models matching the request's inferred capability,
// or all models when nothing matches.
func (r *Router) capabilityPool(req *models.CompletionRequest) []*models.ModelConfig {
	if pool := r.loader.FindModelsByCapability(r.config, r.extractCapability(req)); len(pool) > 0 {
		return pool
	}
	return r.allModels()
}

// healthyPool filters to healthy backends; if none are healthy it returns
// the full pool (best effort beats a guaranteed 503).
func (r *Router) healthyPool(pool []*models.ModelConfig) []*models.ModelConfig {
	var healthy []*models.ModelConfig
	for _, m := range pool {
		if r.isHealthy(m.ModelName) {
			healthy = append(healthy, m)
		}
	}
	if len(healthy) == 0 {
		return pool
	}
	return healthy
}

// orderByPriority sorts candidates by priority (lower = preferred) and
// shuffles within equal-priority groups for load balancing.
func (r *Router) orderByPriority(candidates []*models.ModelConfig) []*models.RoutingDecision {
	sorted := make([]*models.ModelConfig, len(candidates))
	copy(sorted, candidates)

	sort.SliceStable(sorted, func(i, j int) bool {
		return priorityOf(sorted[i]) < priorityOf(sorted[j])
	})
	for i := 0; i < len(sorted); {
		j := i
		for j < len(sorted) && priorityOf(sorted[j]) == priorityOf(sorted[i]) {
			j++
		}
		group := sorted[i:j]
		rand.Shuffle(len(group), func(a, b int) { group[a], group[b] = group[b], group[a] })
		i = j
	}

	decisions := make([]*models.RoutingDecision, 0, len(sorted))
	for _, m := range sorted {
		if dec := r.selectModel(m.ModelName); dec != nil {
			decisions = append(decisions, dec)
		}
	}
	return decisions
}

// extractCapability determines the required capability from the request.
func (r *Router) extractCapability(req *models.CompletionRequest) string {
	if req.Options != nil {
		if cap, ok := req.Options["capability"].(string); ok {
			return cap
		}
	}

	modelName := strings.ToLower(req.Model)
	switch {
	case strings.Contains(modelName, "embed"):
		return "embeddings"
	case strings.Contains(modelName, "vision") || strings.Contains(modelName, "image"):
		return "vision"
	case strings.Contains(modelName, "audio") || strings.Contains(modelName, "whisper"):
		return "audio"
	case strings.Contains(modelName, "code"):
		return "code-generation"
	default:
		return "chat"
	}
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
		Priority:  priorityOf(model),
		Tags:      model.ModelInfo.Tags,
		Reason:    "priority",
	}
}

func priorityOf(m *models.ModelConfig) int {
	if m.ModelInfo != nil && m.ModelInfo.Priority > 0 {
		return m.ModelInfo.Priority
	}
	return 999
}

func (r *Router) allModels() []*models.ModelConfig {
	out := make([]*models.ModelConfig, len(r.config.ModelList))
	for i := range r.config.ModelList {
		out[i] = &r.config.ModelList[i]
	}
	return out
}

func (r *Router) isHealthy(modelName string) bool {
	h, ok := r.health[modelName]
	return !ok || h.Healthy
}

// RecordSuccess marks a backend healthy and resets its error count.
func (r *Router) RecordSuccess(modelName string, responseTimeMs int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	h := r.healthEntry(modelName)
	h.LastCheck = time.Now()
	h.Healthy = true
	h.ErrorCount = 0
	h.ResponseTime = responseTimeMs
}

// RecordFailure counts a 5xx or connection error; after FailureThreshold
// consecutive failures the backend is marked unhealthy.
func (r *Router) RecordFailure(modelName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	h := r.healthEntry(modelName)
	h.LastCheck = time.Now()
	h.ErrorCount++
	if h.ErrorCount >= FailureThreshold {
		h.Healthy = false
	}
}

func (r *Router) healthEntry(modelName string) *ModelHealth {
	if h, ok := r.health[modelName]; ok {
		return h
	}
	h := &ModelHealth{ModelName: modelName, Healthy: true}
	r.health[modelName] = h
	return h
}

// SetStickySession pins a session to a backend for stickyTTL.
func (r *Router) SetStickySession(sessionID, modelName string) {
	r.muSticky.Lock()
	defer r.muSticky.Unlock()

	if len(r.stickyKeys) >= stickyMaxEntries {
		now := time.Now()
		for id, e := range r.stickyKeys {
			if now.After(e.expires) {
				delete(r.stickyKeys, id)
			}
		}
		// Still full: evict arbitrary entries (Go map iteration is random).
		for id := range r.stickyKeys {
			delete(r.stickyKeys, id)
			if len(r.stickyKeys) < stickyMaxEntries {
				break
			}
		}
	}

	r.stickyKeys[sessionID] = stickyEntry{
		model:   modelName,
		expires: time.Now().Add(stickyTTL),
	}
}

func (r *Router) getStickyModel(sessionID string) string {
	r.muSticky.RLock()
	e, ok := r.stickyKeys[sessionID]
	r.muSticky.RUnlock()
	if !ok || time.Now().After(e.expires) {
		return ""
	}
	return e.model
}

// GetModelStatus returns the status of all configured models.
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

func (r *Router) getStatusString(health *ModelHealth) string {
	if health.Healthy {
		switch {
		case health.ResponseTime < 100:
			return "healthy_fast"
		case health.ResponseTime < 500:
			return "healthy_normal"
		default:
			return "healthy_slow"
		}
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
