package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/santura-dev/inference-proxy/internal/models"
)

// Loader handles loading and validation of LiteLLM-style configuration.
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
	l.substituteEnvVars(config)
	return config, nil
}

// envVarPattern matches LiteLLM-style "os.environ.SOME_VAR" references.
var envVarPattern = regexp.MustCompile(`os\.environ\.([A-Za-z_][A-Za-z0-9_]*)`)

// substituteEnvVars replaces "os.environ.VAR_NAME" references with values
// from the environment. Unresolved references are left as-is and treated
// as unset downstream.
func (l *Loader) substituteEnvVars(config *models.LiteLLMConfig) {
	for i := range config.ModelList {
		config.ModelList[i].LiteLLMParams.APIKey = expandEnv(config.ModelList[i].LiteLLMParams.APIKey)
	}
	config.GeneralSettings.MasterKey = expandEnv(config.GeneralSettings.MasterKey)
}

func expandEnv(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		name := strings.TrimPrefix(match, "os.environ.")
		if v := os.Getenv(name); v != "" {
			return v
		}
		return match
	})
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

// LoadDefault loads configuration from the default config.yaml locations.
func LoadDefault() (*models.LiteLLMConfig, error) {
	locations := []string{
		"config.yaml",
		"./config.yaml",
		"/etc/inference-proxy/config.yaml",
		filepath.Join(os.Getenv("HOME"), ".inference-proxy", "config.yaml"),
	}

	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			loader := NewLoader(loc)
			return loader.LoadFromEnv()
		}
	}

	return nil, fmt.Errorf("could not find config.yaml in any default location")
}
