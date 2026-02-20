.PHONY: build test lint docker-build docker-push docker-run deploy namespace secrets status logs port-forward clean help

BINARY_NAME := inference-proxy
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DOCKER_REGISTRY := ghcr.io/santura-dev
DOCKER_IMAGE := $(DOCKER_REGISTRY)/inference-proxy
NAMESPACE := inference
K8S_DIR := deployments/k8s

LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -s -w

help:
	@echo "inference-proxy - available targets:"
	@echo ""
	@echo "  build          - Build the binary"
	@echo "  test           - Run tests"
	@echo "  lint           - Run linters (requires golangci-lint)"
	@echo "  docker-build   - Build Docker image"
	@echo "  docker-push    - Push Docker image to registry"
	@echo "  docker-run     - Run Docker container locally"
	@echo "  deploy         - Deploy the proxy to Kubernetes"
	@echo "  namespace      - Create Kubernetes namespace"
	@echo "  secrets        - Create Kubernetes secrets"
	@echo "  status         - Check deployment status"
	@echo "  logs           - View proxy logs"
	@echo "  port-forward   - Port forward to the proxy"
	@echo "  clean          - Clean build artifacts"

build:
	@echo "Building $(BINARY_NAME) v$(VERSION)..."
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) ./cmd/inference-proxy/
	@echo "Built: bin/$(BINARY_NAME)"

build-multiplatform:
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME)-linux-amd64 ./cmd/inference-proxy/
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME)-linux-arm64 ./cmd/inference-proxy/

test:
	go test -v ./...

lint:
	golangci-lint run ./...

docker-build:
	docker build -t $(DOCKER_IMAGE):$(VERSION) -t $(DOCKER_IMAGE):latest .

docker-buildx:
	docker buildx build --platform linux/amd64,linux/arm64 -t $(DOCKER_IMAGE):$(VERSION) -t $(DOCKER_IMAGE):latest --push .

docker-push:
	docker push $(DOCKER_IMAGE):$(VERSION)
	docker push $(DOCKER_IMAGE):latest

docker-run:
	docker run -p 4000:4000 \
		-e INFERENCE_PROXY_CONFIG=/etc/inference-proxy/config.yaml \
		-e INFERENCE_PROXY_MASTER_KEY=dev-key \
		-v $(PWD)/config.yaml:/etc/inference-proxy/config.yaml:ro \
		$(DOCKER_IMAGE):latest

namespace:
	kubectl apply -f $(K8S_DIR)/namespace/

secrets:
	@if [ -z "$$INFERENCE_PROXY_MASTER_KEY" ]; then echo "Error: INFERENCE_PROXY_MASTER_KEY not set"; exit 1; fi
	kubectl create secret generic inference-secrets \
		--from-literal=INFERENCE_PROXY_MASTER_KEY=$$INFERENCE_PROXY_MASTER_KEY \
		-n $(NAMESPACE) --dry-run=client -o yaml | kubectl apply -f -

deploy: docker-build docker-push namespace secrets
	kubectl apply -f $(K8S_DIR)/proxy/
	kubectl rollout status deployment/inference-proxy -n $(NAMESPACE)

status:
	kubectl get pods -n $(NAMESPACE) -l app=inference-proxy
	kubectl get svc -n $(NAMESPACE)
	kubectl get ingress -n $(NAMESPACE)

logs:
	kubectl logs -n $(NAMESPACE) -l app=inference-proxy -f

port-forward:
	kubectl port-forward -n $(NAMESPACE) svc/inference-proxy 4000:4000

clean:
	rm -rf bin/
	go clean
