#!/bin/bash
# LiteLLM Backend Deployment Script
# Deploys all model servers and the LiteLLM proxy

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="litellm"
K8S_DIR="$SCRIPT_DIR/../deployments/k8s"

echo "============================================"
echo "  LiteLLM Backend Deployment"
echo "============================================"
echo ""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."
    
    local missing=()
    
    if ! command -v kubectl &> /dev/null; then
        missing+=("kubectl")
    fi
    
    if ! command -v docker &> /dev/null; then
        missing+=("docker")
    fi
    
    if [ ${#missing[@]} -ne 0 ]; then
        log_error "Missing prerequisites: ${missing[*]}"
        exit 1
    fi
    
    log_info "All prerequisites met"
}

# Check cluster connectivity
check_cluster() {
    log_info "Checking Kubernetes cluster connectivity..."
    
    if ! kubectl cluster-info &> /dev/null; then
        log_error "Cannot connect to Kubernetes cluster"
        exit 1
    fi
    
    log_info "Cluster connection successful"
}

# Create namespace and base resources
deploy_namespace() {
    log_info "Creating namespace and base resources..."
    kubectl apply -f "$K8S_DIR/namespace/"
    log_info "Namespace ready"
}

# Deploy vLLM servers
deploy_vllm() {
    log_info "Deploying vLLM servers..."
    
    echo "  - Audio (NVIDIA Parakeet)"
    kubectl apply -f "$K8S_DIR/vllm/01-audio.yaml"
    
    echo "  - LLM (Qwen3-30B)"
    kubectl apply -f "$K8S_DIR/vllm/02-llm.yaml"
    
    echo "  - Vision (Qwen3-VL)"
    kubectl apply -f "$K8S_DIR/vllm/03-vision.yaml"
    
    log_info "vLLM servers deployed"
}

# Deploy SGLang servers
deploy_sglang() {
    log_info "Deploying SGLang servers..."
    
    echo "  - LLM (Mistral Large 3)"
    kubectl apply -f "$K8S_DIR/sglang/01-llm.yaml"
    
    echo "  - Embedding (BGE M3)"
    kubectl apply -f "$K8S_DIR/sglang/02-embed.yaml"
    
    log_info "SGLang servers deployed"
}

# Deploy LiteLLM proxy
deploy_proxy() {
    log_info "Deploying LiteLLM proxy..."
    kubectl apply -f "$K8S_DIR/proxy/01-deployment.yaml"
    log_info "Proxy deployed"
}

# Wait for deployments
wait_for_deployments() {
    log_info "Waiting for deployments to be ready..."
    
    local deployments=(
        "vllm-audio"
        "vllm-llm"
        "vllm-vision"
        "sglang-llm"
        "sglang-embed"
        "litellm-proxy"
    )
    
    for deploy in "${deployments[@]}"; do
        echo -n "  Waiting for $deploy..."
        if kubectl wait --for=condition=available \
            --timeout=300s deployment/$deploy -n $NAMESPACE 2>/dev/null; then
            echo " ✓"
        else
            echo " ✗ (timeout)"
            log_warn "Deployment $deploy not ready"
        fi
    done
}

# Show status
show_status() {
    log_info "Deployment status:"
    echo ""
    kubectl get pods -n $NAMESPACE -l "app in (vllm-audio,vllm-llm,vllm-vision,sglang-llm,sglang-embed,litellm-proxy)"
    echo ""
    kubectl get svc -n $NAMESPACE
}

# Main deployment flow
main() {
    echo ""
    
    check_prerequisites
    check_cluster
    
    echo ""
    log_info "Starting deployment..."
    echo ""
    
    deploy_namespace
    echo ""
    
    deploy_vllm
    echo ""
    
    deploy_sglang
    echo ""
    
    deploy_proxy
    echo ""
    
    wait_for_deployments
    echo ""
    
    show_status
    
    echo ""
    log_info "Deployment complete!"
    log_info "Access the proxy at: http://litellm-proxy.$NAMESPACE.svc.cluster.local:4000"
}

# Handle script arguments
case "${1:-deploy}" in
    deploy)
        main
        ;;
    namespace)
        check_cluster
        deploy_namespace
        ;;
    vllm)
        check_cluster
        deploy_vllm
        ;;
    sglang)
        check_cluster
        deploy_sglang
        ;;
    proxy)
        check_cluster
        deploy_proxy
        ;;
    status)
        check_cluster
        show_status
        ;;
    logs)
        kubectl logs -n $NAMESPACE -l app=litellm-proxy -f
        ;;
    port-forward)
        kubectl port-forward -n $NAMESPACE svc/litellm-proxy 4000:4000
        ;;
    help|--help|-h)
        echo "Usage: $0 [command]"
        echo ""
        echo "Commands:"
        echo "  deploy      Full deployment (default)"
        echo "  namespace   Create namespace only"
        echo "  vllm        Deploy vLLM servers only"
        echo "  sglang      Deploy SGLang servers only"
        echo "  proxy       Deploy proxy only"
        echo "  status      Show deployment status"
        echo "  logs        Stream proxy logs"
        echo "  port-forward  Port forward to proxy"
        echo "  help        Show this help"
        ;;
esac
