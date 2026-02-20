package main

import (
	"fmt"
	"log"
	"os"

	"github.com/santura-dev/inference-proxy/api"
	"github.com/santura-dev/inference-proxy/internal/config"
	"github.com/santura-dev/inference-proxy/internal/gateway"
)

func main() {
	configPath := "config.yaml"
	if envPath := os.Getenv("INFERENCE_PROXY_CONFIG"); envPath != "" {
		configPath = envPath
	}

	loader := config.NewLoader(configPath)
	cfg, err := loader.LoadFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	router := gateway.NewRouter(cfg)
	log.Printf("inference-proxy initialized with %d models", router.GetModelCount())

	addr := ":4000"
	if envAddr := os.Getenv("INFERENCE_PROXY_ADDR"); envAddr != "" {
		addr = envAddr
	}

	server := api.NewServer(router, cfg)
	if err := server.Start(addr); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
