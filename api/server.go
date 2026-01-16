package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/santura-dev/litellm-backend/internal/config"
	"github.com/santura-dev/litellm-backend/internal/gateway"
	"github.com/santura-dev/litellm-backend/internal/models"
)

var (
	requestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "litellm_requests_total",
			Help: "Total number of requests",
		},
		[]string{"model", "status"},
	)
	requestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "litellm_request_duration_seconds",
			Help:    "Request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"model"},
	)
	activeRequests = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "litellm_active_requests",
			Help: "Number of active requests",
		},
		[]string{"model"},
	)
)

func init() {
	prometheus.MustRegister(requestsTotal, requestDuration, activeRequests)
}

type Server struct {
	router  *chi.Mux
	gateway *gateway.Router
	config  *models.LiteLLMConfig
	server  *http.Server
}

func NewServer(gw *gateway.Router, cfg *models.LiteLLMConfig) *Server {
	s := &Server{
		gateway: gw,
		config:  cfg,
	}

	s.setupRouter()
	return s
}

func (s *Server) setupRouter() {
	s.router = chi.NewRouter()

	cors := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-API-Key"},
		ExposedHeaders:   []string{"X-Request-ID"},
		AllowCredentials: false,
		MaxAge:           300,
	})
	s.router.Use(cors.Handler)

	s.router.Use(requestLogger)
	s.router.Use(rateLimiter)

	s.router.Get("/health", s.healthHandler)
	s.router.Get("/ready", s.readyHandler)
	s.router.Get("/metrics", promhttp.Handler().ServeHTTP)

	s.router.Route("/v1", func(r chi.Router) {
		r.Post("/chat/completions", s.chatCompletionsHandler)
		r.Post("/embeddings", s.embeddingsHandler)
		r.Post("/completions", s.completionsHandler)
		r.Get("/models", s.modelsHandler)
	})

	s.router.Get("/models", s.modelsHandler)
	s.router.Get("/models/{model_name}", s.modelHandler)
	s.router.Get("/status", s.statusHandler)
}

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

func (s *Server) readyHandler(w http.ResponseWriter, r *http.Request) {
	status := s.gateway.GetModelStatus()
	unhealthy := 0
	for _, m := range status {
		if m.Status == "unhealthy" {
			unhealthy++
		}
	}

	if unhealthy > len(status)/2 {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "degraded",
			"healthy":   len(status) - unhealthy,
			"unhealthy": unhealthy,
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ready",
		"models":  len(status),
		"healthy": len(status) - unhealthy,
	})
}

func (s *Server) modelsHandler(w http.ResponseWriter, r *http.Request) {
	models := s.gateway.GetConfig().ModelList
	response := map[string]interface{}{
		"object": "list",
		"data":   make([]map[string]interface{}, 0, len(models)),
	}

	for _, m := range models {
		modelData := map[string]interface{}{
			"id":       m.ModelName,
			"object":   "model",
			"created":  time.Now().Unix(),
			"owned_by": "santura-dev",
		}
		if m.ModelInfo != nil {
			modelData["description"] = m.ModelInfo.Description
			modelData["capabilities"] = m.ModelInfo.Capabilities
		}
		response["data"] = append(response["data"].([]map[string]interface{}), modelData)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) modelHandler(w http.ResponseWriter, r *http.Request) {
	modelName := chi.URLParam(r, "model_name")
	loader := config.NewLoader("")
	model := loader.FindModel(s.gateway.GetConfig(), modelName)

	if model == nil {
		http.Error(w, fmt.Sprintf("model '%s' not found", modelName), http.StatusNotFound)
		return
	}

	response := map[string]interface{}{
		"id":      model.ModelName,
		"object":  "model",
		"created": time.Now().Unix(),
	}
	if model.ModelInfo != nil {
		response["description"] = model.ModelInfo.Description
		response["capabilities"] = model.ModelInfo.Capabilities
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) statusHandler(w http.ResponseWriter, r *http.Request) {
	status := s.gateway.GetModelStatus()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (s *Server) chatCompletionsHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	var req models.CompletionRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	decision := s.gateway.RouteRequest(&req)
	if decision == nil {
		http.Error(w, "no available model", http.StatusServiceUnavailable)
		return
	}

	activeRequests.WithLabelValues(decision.ModelName).Inc()
	defer activeRequests.WithLabelValues(decision.ModelName).Dec()

	log.Printf("Routing request to %s (api_base: %s)", decision.ModelName, decision.APIBase)

	requestsTotal.WithLabelValues(decision.ModelName, "requested").Inc()
	defer func() {
		requestDuration.WithLabelValues(decision.ModelName).Observe(time.Since(start).Seconds())
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":      fmt.Sprintf("cmpl-%d", time.Now().UnixNano()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   decision.ModelName,
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]string{
					"role":    "assistant",
					"content": fmt.Sprintf("[Routed to %s] This is a placeholder response. Connect to model server at %s", decision.ModelName, decision.APIBase),
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]int{
			"prompt_tokens":     10,
			"completion_tokens": 20,
			"total_tokens":      30,
		},
	})
}

func (s *Server) embeddingsHandler(w http.ResponseWriter, r *http.Request) {
	var req models.EmbeddingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	decision := s.gateway.RouteEmbedding(&req)
	if decision == nil {
		http.Error(w, "no available embedding model", http.StatusServiceUnavailable)
		return
	}

	response := map[string]interface{}{
		"object": "list",
		"data":   make([]map[string]interface{}, 0, len(req.Input)),
		"usage": map[string]int{
			"prompt_tokens": 100,
			"total_tokens":  100,
		},
	}

	for i, _ := range req.Input {
		response["data"] = append(response["data"].([]map[string]interface{}), map[string]interface{}{
			"object":    "embedding",
			"index":     i,
			"embedding": make([]float32, 384),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) completionsHandler(w http.ResponseWriter, r *http.Request) {
	s.chatCompletionsHandler(w, r)
}

func (s *Server) Start(addr string) error {
	s.server = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("Starting LiteLLM API server on %s", addr)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return s.server.Shutdown(ctx)
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

func rateLimiter(next http.Handler) http.Handler {
	return next
}
