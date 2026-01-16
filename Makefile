.PHONY: build test lint docker-build docker-push deploy clean help

# Variables
BINARY_NAME := litellm-proxy
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DOCKER_REGISTRY := ghcr.io/santura-dev
DOCKER_IMAGE := $(DOCKER_REGISTRY)/litellm-backend
NAMESPACE := litellm
K8S_DIR := deployments/k8s

# Go build variables
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -s -w

# Default target
help:
	@echo "LiteLLM Backend - Available targets:"
	@echo ""
	@echo "  build          - Build the binary"
	@echo "  test           - Run tests"
	@echo "  lint           - Run linters (requires golangci-lint)"
	@echo "  docker-build   - Build Docker image"
	@echo "  docker-push    - Push Docker image to registry"
	@echo "  docker-run     - Run Docker container locally"
	@echo "  deploy         - Deploy to Kubernetes"
	@echo "  deploy-ome     - Deploy OME CRDs"
	@echo "  namespace      - Create Kubernetes namespace"
	@echo "  secrets        - Create Kubernetes secrets"
	@echo "  status         - Check deployment status"
	@echo "  logs           - View proxy logs"
	@echo "  port-forward   - Port forward to proxy"
	@echo "  clean          - Clean build artifacts"

# Build the binary
build:
	@echo "Building $(BINARY_NAME) v$(VERSION)..."
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) ./cmd/litellm-proxy/
	@echo "Built: bin/$(BINARY_NAME)"

# Build for multiple platforms
build-multiplatform:
	@echo "Building for multiple platforms..."
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME)-linux-amd64 ./cmd/litellm-proxy/
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME)-linux-arm64 ./cmd/litellm-proxy/
	@echo "Built: bin/$(BINARY_NAME)-*"

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...
	@echo "Tests passed!"

# Run linter
lint:
	@echo "Running linter..."
	golangci-lint run ./...
	@echo "Linting passed!"

# Build Docker image
docker-build:
	@echo "Building Docker image $(DOCKER_IMAGE):$(VERSION)..."
	docker build -t $(DOCKER_IMAGE):$(VERSION) -t $(DOCKER_IMAGE):latest .
	@echo "Built: $(DOCKER_IMAGE):$(VERSION)"

# Build Docker image with buildx (multi-platform)
docker-buildx:
	@echo "Building Docker image with buildx..."
	docker buildx build --platform linux/amd64,linux/arm64 -t $(DOCKER_IMAGE):$(VERSION) -t $(DOCKER_IMAGE):latest --push .

# Push Docker image
docker-push:
	@echo "Pushing $(DOCKER_IMAGE):$(VERSION)..."
	docker push $(DOCKER_IMAGE):$(VERSION)
	docker push $(DOCKER_IMAGE):latest
	@echo "Pushed: $(DOCKER_IMAGE):$(VERSION)"

# Run Docker container locally
docker-run:
	@echo "Running Docker container..."
	docker run -p 4000:4000 -p 4001:4001 \
		-e LITELLM_CONFIG=/etc/litellm/config.yaml \
		-e LITELLM_MASTER_KEY=dev-key \
		-v $(PWD)/config.yaml:/etc/litellm/config.yaml:ro \
		$(DOCKER_IMAGE):latest

# Create Kubernetes namespace
namespace:
	@echo "Creating namespace $(NAMESPACE)..."
	kubectl apply -f $(K8S_DIR)/namespace/
	@echo "Namespace $(NAMESPACE) ready"

# Create secrets from environment
secrets:
	@echo "Creating secrets..."
	@if [ -z "$$LITELLM_MASTER_KEY" ]; then echo "Error: LITELLM_MASTER_KEY not set"; exit 1; fi
	kubectl create secret generic litellm-secrets \
		--from-literal=LITELLM_MASTER_KEY=$$LITELLM_MASTER_KEY \
		--from-literal=INTERNAL_API_KEY=$$INTERNAL_API_KEY \
		-n $(NAMESPACE) --dry-run=client -o yaml | kubectl apply -f -
	@echo "Secrets created"

# Deploy to Kubernetes
deploy: docker-build docker-push namespace secrets
	@echo "Deploying to Kubernetes..."
	kubectl apply -f $(K8S_DIR)/proxy/
	kubectl rollout status deployment/litellm-proxy -n $(NAMESPACE)
	@echo "Deployed successfully!"

# Deploy OME CRDs
deploy-ome:
	@echo "Deploying OME CRDs..."
	kubectl apply -f deployments/ome/
	@echo "OME CRDs deployed"

# Check deployment status
status:
	@echo "Checking deployment status..."
	@echo "=== Pods ==="
	kubectl get pods -n $(NAMESPACE) -l app=litellm-proxy
	@echo ""
	@echo "=== Services ==="
	kubectl get svc -n $(NAMESPACE)
	@echo ""
	@echo "=== Ingress ==="
	kubectl get ingress -n $(NAMESPACE)

# View logs
logs:
	@echo "Viewing logs..."
	kubectl logs -n $(NAMESPACE) -l app=litellm-proxy -f

# Port forward
port-forward:
	@echo "Port forwarding to litellm-proxy..."
	kubectl port-forward -n $(NAMESPACE) svc/litellm-proxy 4000:4000 4001:4001

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf bin/
	go clean
	@echo "Cleaned"

# Generate kubeconfig for development
kubeconfig:
	@echo "Generating kubeconfig..."
	kubectl config view --flatten > kubeconfig.yaml
	@echo "Kubeconfig saved to kubeconfig.yaml"
