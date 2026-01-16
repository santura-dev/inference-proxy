package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/santura-dev/litellm-backend/internal/models"
)

// Loader handles loading and validation of LiteLLM configuration.
type Loader struct {
	configPath string
}

// NewLoader creates a new configuration loader.
func NewLoader(configPath string) *Loader {
	return &Loader{configPath: configPath}
}

// Load loads and validates the configuration from the specified path.
func (l *Loader) Load() (*models.LiteLLMConfig, error) {
	data, err := os.ReadFile(l.configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config models.LiteLLMConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Validate configuration
	if err := l.validate(&config); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return &config, nil
}

// validate performs validation on the loaded configuration.
func (l *Loader) validate(config *models.LiteLLMConfig) error {
	if len(config.ModelList) == 0 {
		return fmt.Errorf("model_list cannot be empty")
	}

	seenModels := make(map[string]bool)
	for i, model := range config.ModelList {
		if model.ModelName == "" {
			return fmt.Errorf("model at index %d has empty model_name", i)
		}
		if model.LiteLLMParams.Model == "" {
			return fmt.Errorf("model '%s' has empty model in litellm_params", model.ModelName)
		}

		if seenModels[model.ModelName] {
			return fmt.Errorf("duplicate model name: %s", model.ModelName)
		}
		seenModels[model.ModelName] = true
	}

	return nil
}

// LoadFromEnv loads configuration with environment variable substitution.
func (l *Loader) LoadFromEnv() (*models.LiteLLMConfig, error) {
	config, err := l.Load()
	if err != nil {
		return nil, err
	}

	// Apply environment variable substitutions
	l.substituteEnvVars(config)

	return config, nil
}

// substituteEnvVars replaces template variables with environment values.
func (l *Loader) substituteEnvVars(config *models.LiteLLMConfig) {
	for i := range config.ModelList {
		model := &config.ModelList[i]
		if model.LiteLLMParams.APIKey != "" && model.LiteLLMParams.APIKey == "os.environ.INTERNAL_API_KEY" {
			if key := os.Getenv("INTERNAL_API_KEY"); key != "" {
				model.LiteLLMParams.APIKey = key
			}
		}
		if model.LiteLLMParams.APIKey != "" && model.LiteLLMParams.APIKey == "os.environ.ANTHROPIC_API_KEY" {
			if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
				model.LiteLLMParams.APIKey = key
			}
		}
		if model.LiteLLMParams.APIKey != "" && model.LiteLLMParams.APIKey == "os.environ.OPENAI_API_KEY" {
			if key := os.Getenv("OPENAI_API_KEY"); key != "" {
				model.LiteLLMParams.APIKey = key
			}
		}
	}
}

// FindModel finds a model configuration by name.
func (l *Loader) FindModel(config *models.LiteLLMConfig, modelName string) *models.ModelConfig {
	for i := range config.ModelList {
		if config.ModelList[i].ModelName == modelName {
			return &config.ModelList[i]
		}
	}
	return nil
}

// FindModelsByCapability finds all models with the specified capability.
func (l *Loader) FindModelsByCapability(config *models.LiteLLMConfig, capability string) []*models.ModelConfig {
	var result []*models.ModelConfig
	for i := range config.ModelList {
		model := &config.ModelList[i]
		if model.ModelInfo != nil {
			for _, cap := range model.ModelInfo.Capabilities {
				if cap == capability {
					result = append(result, model)
					break
				}
			}
		}
	}
	return result
}

// FindModelsByOwner finds all models owned by the specified team.
func (l *Loader) FindModelsByOwner(config *models.LiteLLMConfig, owner string) []*models.ModelConfig {
	var result []*models.ModelConfig
	for i := range config.ModelList {
		model := &config.ModelList[i]
		if model.ModelInfo != nil && model.ModelInfo.Owner == owner {
			result = append(result, model)
		}
	}
	return result
}

// GetModelsByPriority returns models sorted by priority.
func (l *Loader) GetModelsByPriority(config *models.LiteLLMConfig) []*models.ModelConfig {
	models := make([]*models.ModelConfig, len(config.ModelList))
	for i := range config.ModelList {
		models[i] = &config.ModelList[i]
	}

	// Sort by priority (lower number = higher priority)
	// For models without priority, default to 999
	for i := range models {
		if models[i].ModelInfo != nil && models[i].ModelInfo.Priority == 0 {
			models[i].ModelInfo.Priority = 999
		}
	}

	// Simple insertion sort for small lists
	for i := 1; i < len(models); i++ {
		key := models[i]
		j := i - 1
		for j >= 0 {
			keyPriority := 999
			if key.ModelInfo != nil {
				keyPriority = key.ModelInfo.Priority
			}
			currentPriority := 999
			if models[j].ModelInfo != nil {
				currentPriority = models[j].ModelInfo.Priority
			}
			if currentPriority <= keyPriority {
				break
			}
			models[j+1] = models[j]
			j--
		}
		models[j+1] = key
	}

	return models
}

// GetModelNames returns a list of all model names.
func (l *Loader) GetModelNames(config *models.LiteLLMConfig) []string {
	names := make([]string, len(config.ModelList))
	for i, model := range config.ModelList {
		names[i] = model.ModelName
	}
	return names
}

// LoadDefault loads configuration from the default config.yaml location.
func LoadDefault() (*models.LiteLLMConfig, error) {
	// Try multiple common locations
	locations := []string{
		"config.yaml",
		"./config.yaml",
		"/etc/litellm/config.yaml",
		filepath.Join(os.Getenv("HOME"), ".litellm", "config.yaml"),
	}

	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			loader := NewLoader(loc)
			return loader.LoadFromEnv()
		}
	}

	return nil, fmt.Errorf("could not find config.yaml in any default location")
}
