package main

import (
	"fmt"
	"log"
	"os"

	"github.com/santura-dev/litellm-backend/api"
	"github.com/santura-dev/litellm-backend/internal/config"
	"github.com/santura-dev/litellm-backend/internal/gateway"
)

func main() {
	configPath := "config.yaml"
	if envPath := os.Getenv("LITELLM_CONFIG"); envPath != "" {
		configPath = envPath
	}

	loader := config.NewLoader(configPath)
	cfg, err := loader.LoadFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	router := gateway.NewRouter(cfg)
	log.Printf("LiteLLM Backend initialized with %d models", router.GetModelCount())

	status := router.GetModelStatus()
	for name, s := range status {
		log.Printf("  - %s: %s (latency: %dms)", name, s.Status, s.ResponseTime)
	}

	addr := ":4000"
	if envAddr := os.Getenv("LITELLM_ADDR"); envAddr != "" {
		addr = envAddr
	}

	server := api.NewServer(router, cfg)
	if err := server.Start(addr); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
