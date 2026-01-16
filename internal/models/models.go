package models

import "time"

type LiteLLMConfig struct {
	ModelList       []ModelConfig   `yaml:"model_list"`
	GeneralSettings GeneralSettings `yaml:"general_settings,omitempty"`
	LitellmSettings LitellmSettings `yaml:"litellm_settings,omitempty"`
}

type ModelConfig struct {
	ModelName     string        `yaml:"model_name"`
	LiteLLMParams LiteLLMParams `yaml:"litellm_params"`
	ModelInfo     *ModelInfo    `yaml:"model_info,omitempty"`
}

type LiteLLMParams struct {
	Model   string `yaml:"model"`
	APIBase string `yaml:"api_base,omitempty"`
	APIKey  string `yaml:"api_key,omitempty"`
}

type ModelInfo struct {
	Mode         string   `yaml:"mode"`
	Description  string   `yaml:"description,omitempty"`
	Owner        string   `yaml:"owner,omitempty"`
	Capabilities []string `yaml:"capabilities,omitempty"`
	Priority     int      `yaml:"priority,omitempty"`
	Tags         []string `yaml:"tags,omitempty"`
}

type GeneralSettings struct {
	MasterKey      string `yaml:"master_key,omitempty"`
	StreamTimeout  int    `yaml:"stream_timeout,omitempty"`
	MaxRetries     int    `yaml:"max_retries,omitempty"`
	RetryOnFailure bool   `yaml:"retry_on_failure,omitempty"`
	AllowRequests  bool   `yaml:"allow_requests,omitempty"`
}

type LitellmSettings struct {
	CacheType          string `yaml:"cache_type,omitempty"`
	CacheExpirySeconds int    `yaml:"cache_expiry_seconds,omitempty"`
}

type CompletionRequest struct {
	Model    string                 `json:"model"`
	Messages []Message              `json:"messages,omitempty"`
	Prompt   string                 `json:"prompt,omitempty"`
	Stream   bool                   `json:"stream,omitempty"`
	Options  map[string]interface{} `json:"options,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Name    string `json:"name,omitempty"`
}

type CompletionResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason,omitempty"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type EmbeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type EmbeddingResponse struct {
	Object string          `json:"object"`
	Data   []EmbeddingData `json:"data"`
	Usage  EmbeddingUsage  `json:"usage"`
}

type EmbeddingData struct {
	Object    string    `json:"object"`
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}

type EmbeddingUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type ModelStatus struct {
	ModelName    string    `json:"model_name"`
	Status       string    `json:"status"`
	LastCheck    time.Time `json:"last_check"`
	ResponseTime int64     `json:"response_time_ms,omitempty"`
}

type RoutingDecision struct {
	ModelName string   `json:"model_name"`
	APIBase   string   `json:"api_base"`
	Priority  int      `json:"priority"`
	Tags      []string `json:"tags,omitempty"`
	Reason    string   `json:"reason,omitempty"`
}
