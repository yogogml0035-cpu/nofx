package mcp

import (
	"net/http"
	"os"
	"strconv"
	"time"

	"nofx/logger"
	"nofx/security"
)

// Config client configuration (centralized management of all configurations)
type Config struct {
	// Provider configuration
	Provider string
	APIKey   string
	BaseURL  string
	Model    string

	// Behavior configuration
	MaxTokens       int
	MaxContext      int // Model's max context window in tokens (0 = no limit)
	Temperature     float64
	UseFullURL      bool
	ReasoningEffort string // Canonical effort: low/medium/high/xhigh/max ("" = provider default)

	// Retry configuration
	MaxRetries      int
	RetryWaitBase   time.Duration
	RetryableErrors []string

	// Timeout configuration
	Timeout time.Duration

	// Dependency injection
	Logger     Logger
	HTTPClient *http.Client
}

// DefaultConfig returns default configuration
func DefaultConfig() *Config {
	return &Config{
		// Thinking models spend part of this budget on reasoning before emitting
		// the answer, so a 2000-token cap can leave Content empty with
		// finish_reason=length. 8000 is the value every deployment compose file
		// already uses; keep the code default in sync so a stack that does not
		// inject AI_MAX_TOKENS still has room for reasoning + decision.
		MaxTokens:       getEnvInt("AI_MAX_TOKENS", 8000),
		Temperature:     MCPClientTemperature,
		MaxRetries:      MaxRetryTimes,
		RetryWaitBase:   2 * time.Second,
		Timeout:         DefaultTimeout,
		RetryableErrors: retryableErrors,

		// Default dependencies (use global logger)
		Logger:     logger.NewMCPLogger(),
		HTTPClient: security.SafeHTTPClient(DefaultTimeout),
	}
}

// getEnvInt reads integer from environment variable, returns default value if failed
func getEnvInt(key string, defaultValue int) int {
	if val := os.Getenv(key); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			return parsed
		}
	}
	return defaultValue
}

// getEnvString reads string from environment variable, returns default value if empty
func getEnvString(key string, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
}
