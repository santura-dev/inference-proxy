#!/bin/bash
# inference-proxy test script
# Tests API endpoints and model connectivity

set -e

PROXY_URL="${INFERENCE_PROXY_URL:-http://localhost:4000}"
NAMESPACE="${NAMESPACE:-litellm}"

echo "============================================"
echo "  inference-proxy tests"
echo "============================================"
echo ""
echo "Proxy URL: $PROXY_URL"
echo ""

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

pass() { echo -e "  ${GREEN}✓${NC} $1"; }
fail() { echo -e "  ${RED}✗${NC} $1"; }
info() { echo -e "  ${YELLOW}[?]${NC} $1"; }

# Test health endpoint
test_health() {
    echo "Testing health endpoint..."
    if curl -sf "${PROXY_URL}/health" > /dev/null; then
        pass "Health check passed"
        return 0
    else
        fail "Health check failed"
        return 1
    fi
}

# Test ready endpoint
test_ready() {
    echo "Testing ready endpoint..."
    if curl -sf "${PROXY_URL}/ready" > /dev/null; then
        pass "Ready check passed"
        return 0
    else
        fail "Ready check failed"
        return 1
    fi
}

# Test models list
test_models() {
    echo "Testing models endpoint..."
    local response
    response=$(curl -sf "${PROXY_URL}/models" 2>/dev/null)
    if [ $? -eq 0 ]; then
        local count
        count=$(echo "$response" | grep -o '"id"' | wc -l)
        pass "Models endpoint returned $count models"
        echo "$response" | head -c 200
        echo "..."
        return 0
    else
        fail "Models endpoint failed"
        return 1
    fi
}

# Test chat completion
test_chat() {
    echo "Testing chat completion..."
    local response
    response=$(curl -sf -X POST "${PROXY_URL}/v1/chat/completions" \
        -H "Content-Type: application/json" \
        -d '{
            "model": "llama-3.1-8b",
            "messages": [{"role": "user", "content": "Say hello"}],
            "max_tokens": 50
        }' 2>/dev/null)
    if [ $? -eq 0 ]; then
        pass "Chat completion succeeded"
        return 0
    else
        fail "Chat completion failed"
        return 1
    fi
}

# Test embeddings
test_embeddings() {
    echo "Testing embeddings..."
    local response
    response=$(curl -sf -X POST "${PROXY_URL}/v1/embeddings" \
        -H "Content-Type: application/json" \
        -d '{
            "model": "bge-m3",
            "input": ["Hello world", "Test embedding"]
        }' 2>/dev/null)
    if [ $? -eq 0 ]; then
        pass "Embeddings succeeded"
        return 0
    else
        fail "Embeddings failed"
        return 1
    fi
}

# Test metrics endpoint
test_metrics() {
    echo "Testing metrics endpoint..."
    if curl -sf "${PROXY_URL}/metrics" > /dev/null; then
        pass "Metrics endpoint accessible"
        return 0
    else
        fail "Metrics endpoint failed"
        return 1
    fi
}

# Test model status
test_status() {
    echo "Testing status endpoint..."
    local response
    response=$(curl -sf "${PROXY_URL}/status" 2>/dev/null)
    if [ $? -eq 0 ]; then
        pass "Status endpoint accessible"
        echo "$response" | head -c 200
        return 0
    else
        fail "Status endpoint failed"
        return 1
    fi
}

# Run all tests
run_tests() {
    local failed=0
    
    test_health || ((failed++))
    echo ""
    
    test_ready || ((failed++))
    echo ""
    
    test_models || ((failed++))
    echo ""
    
    test_status || ((failed++))
    echo ""
    
    test_metrics || ((failed++))
    echo ""
    
    test_chat || ((failed++))
    echo ""
    
    test_embeddings || ((failed++))
    echo ""
    
    echo "============================================"
    if [ $failed -eq 0 ]; then
        echo -e "${GREEN}All tests passed!${NC}"
        return 0
    else
        echo -e "${RED}$failed test(s) failed${NC}"
        return 1
    fi
}

# Port forward helper
port_forward() {
    echo "Starting port forward to proxy..."
    kubectl port-forward -n $NAMESPACE svc/inference-proxy 4000:4000
}

# Show usage
usage() {
    echo "Usage: $0 [command]"
    echo ""
    echo "Commands:"
    echo "  all         Run all tests (default)"
    echo "  health      Test health endpoint"
    echo "  ready       Test ready endpoint"
    echo "  models      Test models list"
    echo "  chat        Test chat completion"
    echo "  embed       Test embeddings"
    echo "  metrics     Test metrics endpoint"
    echo "  status      Test status endpoint"
    echo "  port-forward Start port forward"
    echo "  help        Show help"
}

# Main
case "${1:-all}" in
    all)
        run_tests
        ;;
    health)
        test_health
        ;;
    ready)
        test_ready
        ;;
    models)
        test_models
        ;;
    chat)
        test_chat
        ;;
    embed|embeddings)
        test_embeddings
        ;;
    metrics)
        test_metrics
        ;;
    status)
        test_status
        ;;
    port-forward)
        port_forward
        ;;
    help|--help|-h)
        usage
        ;;
esac
