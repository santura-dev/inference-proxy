package gateway

import (
	"testing"
	"time"

	"github.com/santura-dev/inference-proxy/internal/models"
)

func testRouter() *Router {
	cfg := &models.LiteLLMConfig{
		ModelList: []models.ModelConfig{
			{
				ModelName:     "chat-primary",
				LiteLLMParams: models.LiteLLMParams{Model: "m1", APIBase: "http://primary:8000/v1"},
				ModelInfo:     &models.ModelInfo{Mode: "chat", Capabilities: []string{"chat"}, Priority: 1},
			},
			{
				ModelName:     "chat-backup",
				LiteLLMParams: models.LiteLLMParams{Model: "m2", APIBase: "http://backup:8000/v1"},
				ModelInfo:     &models.ModelInfo{Mode: "chat", Capabilities: []string{"chat"}, Priority: 2},
			},
			{
				ModelName:     "bge-m3",
				LiteLLMParams: models.LiteLLMParams{Model: "m3", APIBase: "http://embed:30000/v1"},
				ModelInfo:     &models.ModelInfo{Mode: "embeddings", Capabilities: []string{"embeddings"}, Priority: 1},
			},
		},
	}
	return NewRouter(cfg)
}

func TestExactModelMatchWins(t *testing.T) {
	r := testRouter()
	decs := r.RouteCandidates(&models.CompletionRequest{Model: "chat-backup"}, "")
	if len(decs) == 0 || decs[0].ModelName != "chat-backup" {
		t.Fatalf("expected exact match to chat-backup, got %+v", decs)
	}
	if len(decs) < 2 || decs[1].ModelName != "chat-primary" {
		t.Fatalf("expected chat-primary as failover candidate after exact match, got %+v", decs)
	}
}

func TestCapabilityRoutingOrdersByPriority(t *testing.T) {
	r := testRouter()
	decs := r.RouteCandidates(&models.CompletionRequest{Model: "some-unknown-model"}, "")
	if len(decs) != 2 || decs[0].ModelName != "chat-primary" || decs[1].ModelName != "chat-backup" {
		t.Fatalf("expected priority-ordered chat candidates, got %+v", decs)
	}
}

func TestUnhealthyBackendLosesPriority(t *testing.T) {
	r := testRouter()
	for i := 0; i < FailureThreshold; i++ {
		r.RecordFailure("chat-primary")
	}
	decs := r.RouteCandidates(&models.CompletionRequest{Model: "unknown-model"}, "")
	if len(decs) == 0 || decs[0].ModelName != "chat-backup" {
		t.Fatalf("expected backup first after primary marked unhealthy, got %+v", decs)
	}
}

func TestSuccessResetsHealth(t *testing.T) {
	r := testRouter()
	for i := 0; i < FailureThreshold; i++ {
		r.RecordFailure("chat-primary")
	}
	r.RecordSuccess("chat-primary", 50)
	decs := r.RouteCandidates(&models.CompletionRequest{Model: "unknown-model"}, "")
	if len(decs) == 0 || decs[0].ModelName != "chat-primary" {
		t.Fatalf("expected primary restored to first after success, got %+v", decs)
	}
}

func TestStickySessionPinsBackend(t *testing.T) {
	r := testRouter()
	r.SetStickySession("s1", "chat-backup")
	decs := r.RouteCandidates(&models.CompletionRequest{Model: "unknown-model"}, "s1")
	if len(decs) != 1 || decs[0].ModelName != "chat-backup" || decs[0].Reason != "sticky_session" {
		t.Fatalf("expected sticky pin to chat-backup, got %+v", decs)
	}
}

func TestStickyExpiredFallsThrough(t *testing.T) {
	r := testRouter()
	r.SetStickySession("s1", "chat-backup")
	r.muSticky.Lock()
	e := r.stickyKeys["s1"]
	e.expires = time.Now().Add(-time.Minute)
	r.stickyKeys["s1"] = e
	r.muSticky.Unlock()

	decs := r.RouteCandidates(&models.CompletionRequest{Model: "unknown-model"}, "s1")
	if len(decs) != 2 || decs[0].ModelName != "chat-primary" {
		t.Fatalf("expected expired sticky to fall through to priority order, got %+v", decs)
	}
}

func TestEmbeddingRoutingExactMatch(t *testing.T) {
	r := testRouter()
	decs := r.RouteEmbeddingCandidates(&models.EmbeddingRequest{Model: "bge-m3", Input: []string{"x"}})
	if len(decs) == 0 || decs[0].ModelName != "bge-m3" {
		t.Fatalf("expected exact embedding match, got %+v", decs)
	}
}

func TestRoutingDecisionCarriesRealPriority(t *testing.T) {
	r := testRouter()
	dec := r.selectModel("chat-primary")
	if dec == nil || dec.Priority != 1 {
		t.Fatalf("expected real priority 1, got %+v", dec)
	}
}
