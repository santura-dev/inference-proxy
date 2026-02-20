package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/santura-dev/inference-proxy/internal/config"
	"github.com/santura-dev/inference-proxy/internal/gateway"
	"github.com/santura-dev/inference-proxy/internal/models"
)

var (
	requestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "inference_proxy_requests_total",
			Help: "Total number of proxied requests",
		},
		[]string{"model", "status"}, // success | upstream_error | no_backend | client_error
	)
	requestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "inference_proxy_request_duration_seconds",
			Help:    "End-to-end request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"model"},
	)
	activeRequests = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "inference_proxy_active_requests",
			Help: "Number of in-flight proxied requests",
		},
		[]string{"model"},
	)
)

func init() {
	prometheus.MustRegister(requestsTotal, requestDuration, activeRequests)
}

const maxRequestBody = 32 << 20

type Server struct {
	router  *chi.Mux
	gateway *gateway.Router
	config  *models.LiteLLMConfig
	server  *http.Server
	client  *http.Client
}

func NewServer(gw *gateway.Router, cfg *models.LiteLLMConfig) *Server {
	s := &Server{
		gateway: gw,
		config:  cfg,
		client: &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:        256,
				MaxIdleConnsPerHost: 64,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
	s.setupRouter()
	return s
}

func (s *Server) setupRouter() {
	s.router = chi.NewRouter()

	s.router.Use(cors.New(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Session-ID"},
		ExposedHeaders:   []string{"X-Request-ID"},
		AllowCredentials: false,
		MaxAge:           300,
	}).Handler)

	s.router.Use(requestLogger)

	s.router.Get("/health", s.healthHandler)
	s.router.Get("/ready", s.readyHandler)
	s.router.Get("/metrics", promhttp.Handler().ServeHTTP)
	s.router.Get("/status", s.statusHandler)

	s.router.Route("/v1", func(r chi.Router) {
		r.Use(s.auth)
		r.Post("/chat/completions", s.chatCompletionsHandler)
		r.Post("/completions", s.completionsHandler)
		r.Post("/embeddings", s.embeddingsHandler)
		r.Get("/models", s.modelsHandler)
	})

	s.router.Get("/models", s.modelsHandler)
	s.router.Get("/models/{model_name}", s.modelHandler)
}

// masterKey returns the configured key, or "" if auth is disabled or the
// env reference in the config was never resolved.
func (s *Server) masterKey() string {
	key := s.config.GeneralSettings.MasterKey
	if key == "" || strings.HasPrefix(key, "os.environ.") {
		return ""
	}
	return key
}

func (s *Server) auth(next http.Handler) http.Handler {
	key := s.masterKey()
	if key == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+key {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid api key"})
			return
		}
		next.ServeHTTP(w, r)
	})
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
	s.proxyLLM(w, r)
}

func (s *Server) completionsHandler(w http.ResponseWriter, r *http.Request) {
	s.proxyLLM(w, r)
}

// proxyLLM routes a completion request and fails over across candidate
// backends. Failover happens until response headers are received; once the
// upstream starts responding (including SSE streams), the response is
// relayed as-is.
func (s *Server) proxyLLM(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBody))
	if err != nil {
		http.Error(w, "request body too large or unreadable", http.StatusRequestEntityTooLarge)
		return
	}

	var req models.CompletionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		requestsTotal.WithLabelValues("unknown", "client_error").Inc()
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	sessionID := r.Header.Get("X-Session-ID")
	if sessionID == "" && req.Options != nil {
		if sid, ok := req.Options["session_id"].(string); ok {
			sessionID = sid
		}
	}

	candidates := s.gateway.RouteCandidates(&req, sessionID)
	if len(candidates) == 0 {
		requestsTotal.WithLabelValues(req.Model, "no_backend").Inc()
		http.Error(w, "no available model", http.StatusServiceUnavailable)
		return
	}

	var lastStatus int
	for _, dec := range candidates {
		status, handled := s.attempt(w, r, dec, body, sessionID, start)
		if handled {
			return
		}
		lastStatus = status
	}

	requestsTotal.WithLabelValues(candidates[0].ModelName, "upstream_error").Inc()
	http.Error(w, fmt.Sprintf("all %d backends failed (last status %d)", len(candidates), lastStatus), http.StatusBadGateway)
}

func (s *Server) embeddingsHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBody))
	if err != nil {
		http.Error(w, "request body too large or unreadable", http.StatusRequestEntityTooLarge)
		return
	}

	var req models.EmbeddingRequest
	if err := json.Unmarshal(body, &req); err != nil {
		requestsTotal.WithLabelValues("unknown", "client_error").Inc()
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	candidates := s.gateway.RouteEmbeddingCandidates(&req)
	if len(candidates) == 0 {
		requestsTotal.WithLabelValues(req.Model, "no_backend").Inc()
		http.Error(w, "no available embedding model", http.StatusServiceUnavailable)
		return
	}

	var lastStatus int
	for _, dec := range candidates {
		status, handled := s.attempt(w, r, dec, body, "", start)
		if handled {
			return
		}
		lastStatus = status
	}

	requestsTotal.WithLabelValues(candidates[0].ModelName, "upstream_error").Inc()
	http.Error(w, fmt.Sprintf("all %d embedding backends failed (last status %d)", len(candidates), lastStatus), http.StatusBadGateway)
}

// attempt proxies one request to one backend. Returns (httpStatus, handled):
// handled=false means the backend failed with a 5xx/connection error and the
// caller should try the next candidate.
func (s *Server) attempt(w http.ResponseWriter, r *http.Request, dec *models.RoutingDecision, body []byte, sessionID string, start time.Time) (int, bool) {
	upstream := upstreamURL(dec.APIBase, r.URL.Path)

	req, err := http.NewRequestWithContext(r.Context(), r.Method, upstream, bytes.NewReader(body))
	if err != nil {
		log.Printf("backend %s: invalid upstream url %s: %v", dec.ModelName, upstream, err)
		return http.StatusBadGateway, false
	}
	copyRequestHeaders(req.Header, r.Header)

	resp, err := s.client.Do(req)
	if err != nil {
		s.gateway.RecordFailure(dec.ModelName)
		log.Printf("backend %s unreachable at %s: %v", dec.ModelName, upstream, err)
		return http.StatusBadGateway, false
	}

	if resp.StatusCode >= 500 {
		s.gateway.RecordFailure(dec.ModelName)
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		log.Printf("backend %s returned %d, failing over", dec.ModelName, resp.StatusCode)
		return resp.StatusCode, false
	}

	s.gateway.RecordSuccess(dec.ModelName, time.Since(start).Milliseconds())

	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	activeRequests.WithLabelValues(dec.ModelName).Inc()
	defer activeRequests.WithLabelValues(dec.ModelName).Dec()

	// Relay the body, flushing as we go so SSE streams arrive token by token.
	flush, _ := w.(http.Flusher)
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				break
			}
			if flush != nil {
				flush.Flush()
			}
		}
		if err != nil {
			break
		}
	}
	resp.Body.Close()

	if sessionID != "" {
		s.gateway.SetStickySession(sessionID, dec.ModelName)
	}
	requestsTotal.WithLabelValues(dec.ModelName, "success").Inc()
	requestDuration.WithLabelValues(dec.ModelName).Observe(time.Since(start).Seconds())
	return resp.StatusCode, true
}

// upstreamURL joins a backend api_base (LiteLLM convention: includes /v1)
// with the request path past the proxy's own /v1 prefix.
func upstreamURL(apiBase, requestPath string) string {
	rest := strings.TrimPrefix(requestPath, "/v1")
	if !strings.HasPrefix(rest, "/") {
		rest = "/" + rest
	}
	return strings.TrimSuffix(apiBase, "/") + rest
}

func copyRequestHeaders(dst, src http.Header) {
	hopByHop := map[string]bool{
		"connection": true, "keep-alive": true, "proxy-authenticate": true,
		"proxy-authorization": true, "te": true, "trailers": true,
		"transfer-encoding": true, "upgrade": true, "host": true, "content-length": true,
	}
	for k, vv := range src {
		if hopByHop[strings.ToLower(k)] {
			continue
		}
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func (s *Server) Start(addr string) error {
	s.server = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // streaming responses must not be cut off
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("inference-proxy listening on %s", addr)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return s.server.Shutdown(ctx)
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if r.URL.Path == "/health" || r.URL.Path == "/ready" || r.URL.Path == "/metrics" {
			return
		}
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}
