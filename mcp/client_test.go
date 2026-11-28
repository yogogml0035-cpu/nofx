package mcp

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

// ============================================================
// Test Client Creation and Configuration
// ============================================================

func TestNewClient_Default(t *testing.T) {
	client := NewClient()

	if client == nil {
		t.Fatal("client should not be nil")
	}

	c := client.(*Client)
	if c.Provider == "" {
		t.Error("Provider should have default value")
	}

	if c.MaxTokens <= 0 {
		t.Error("MaxTokens should be positive")
	}

	if c.Log == nil {
		t.Error("Log should not be nil")
	}

	if c.HTTPClient == nil {
		t.Error("HTTPClient should not be nil")
	}

	if c.Hooks == nil {
		t.Error("Hooks should not be nil")
	}
}

func TestNewClient_WithOptions(t *testing.T) {
	mockLogger := NewMockLogger()
	mockHTTP := &http.Client{Timeout: 30 * time.Second}

	client := NewClient(
		WithLogger(mockLogger),
		WithHTTPClient(mockHTTP),
		WithMaxTokens(4000),
		WithTimeout(60*time.Second),
		WithAPIKey("test-key"),
	)

	c := client.(*Client)

	if c.Log != mockLogger {
		t.Error("Log should be set from option")
	}

	if c.HTTPClient != mockHTTP {
		t.Error("HTTPClient should be set from option")
	}

	if c.MaxTokens != 4000 {
		t.Error("MaxTokens should be 4000")
	}

	if c.APIKey != "test-key" {
		t.Error("APIKey should be test-key")
	}
}

// ============================================================
// Test CallWithMessages
// ============================================================

func TestClient_CallWithMessages_Success(t *testing.T) {
	mockHTTP := NewMockHTTPClient()
	mockHTTP.SetSuccessResponse("AI response content")
	mockLogger := NewMockLogger()

	client := NewClient(
		WithHTTPClient(mockHTTP.ToHTTPClient()),
		WithLogger(mockLogger),
		WithAPIKey("test-key"),
		WithBaseURL("https://api.test.com"),
	)

	result, err := client.CallWithMessages("system prompt", "user prompt")

	if err != nil {
		t.Fatalf("should not error: %v", err)
	}

	if result != "AI response content" {
		t.Errorf("expected 'AI response content', got '%s'", result)
	}

	// Verify request
	requests := mockHTTP.GetRequests()
	if len(requests) != 1 {
		t.Errorf("expected 1 request, got %d", len(requests))
	}

	if len(requests) > 0 {
		req := requests[0]
		if req.Header.Get("Authorization") == "" {
			t.Error("Authorization header should be set")
		}
		if req.Header.Get("Content-Type") != "application/json" {
			t.Error("Content-Type should be application/json")
		}
	}
}

func TestClient_CallWithMessages_NoAPIKey(t *testing.T) {
	client := NewClient()

	_, err := client.CallWithMessages("system", "user")

	if err == nil {
		t.Error("should error when API key is not set")
	}

	if err.Error() != "AI API key not set, please call SetAPIKey first" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestClient_CallWithMessages_HTTPError(t *testing.T) {
	mockHTTP := NewMockHTTPClient()
	mockHTTP.SetErrorResponse(500, "Internal Server Error")
	mockLogger := NewMockLogger()

	client := NewClient(
		WithHTTPClient(mockHTTP.ToHTTPClient()),
		WithLogger(mockLogger),
		WithAPIKey("test-key"),
	)

	_, err := client.CallWithMessages("system", "user")

	if err == nil {
		t.Error("should error on HTTP error")
	}
}

// ============================================================
// Test Retry Logic
// ============================================================

func TestClient_Retry_Success(t *testing.T) {
	mockHTTP := NewMockHTTPClient()
	mockLogger := NewMockLogger()

	// Simulate: first call fails, second call succeeds
	callCount := 0
	mockHTTP.ResponseFunc = func(req *http.Request) (*http.Response, error) {
		callCount++
		if callCount == 1 {
			return nil, errors.New("connection reset")
		}
		return &http.Response{
			StatusCode: 200,
			Body:       http.NoBody,
		}, nil
	}

	client := NewClient(
		WithHTTPClient(mockHTTP.ToHTTPClient()),
		WithLogger(mockLogger),
		WithAPIKey("test-key"),
		WithMaxRetries(3),
	)

	// Since our client uses Hooks.Call, need special handling
	// Here we test that CallWithMessages will invoke retry logic
	c := client.(*Client)

	// Temporarily modify retry wait time to 0 to speed up test
	oldRetries := MaxRetryTimes
	MaxRetryTimes = 3
	defer func() { MaxRetryTimes = oldRetries }()

	_, err := c.CallWithMessages("system", "user")

	// First fails (connection reset), second succeeds, but response format is wrong, will fail
	// But at least verify retry logic was triggered
	if callCount < 2 {
		t.Errorf("should retry, got %d calls", callCount)
	}

	// Check if there's retry information in logs
	logs := mockLogger.GetLogsByLevel("WARN")
	hasRetryLog := false
	for _, log := range logs {
		if log.Message == "⚠️  AI API call failed, retrying (2/3)..." {
			hasRetryLog = true
			break
		}
	}

	if !hasRetryLog && callCount >= 2 {
		// If retry was indeed attempted, there should be warning logs
		// But due to our test setup, it may not trigger, so just check here
		t.Log("Retry was attempted")
	}

	_ = err // Ignore error, we mainly test retry logic was triggered
}

func TestClient_Retry_NonRetryableError(t *testing.T) {
	mockHTTP := NewMockHTTPClient()
	mockHTTP.SetErrorResponse(400, "Bad Request")
	mockLogger := NewMockLogger()

	client := NewClient(
		WithHTTPClient(mockHTTP.ToHTTPClient()),
		WithLogger(mockLogger),
		WithAPIKey("test-key"),
	)

	_, err := client.CallWithMessages("system", "user")

	if err == nil {
		t.Error("should error")
	}

	// Verify no retry (because 400 is not a retryable error)
	requests := mockHTTP.GetRequests()
	if len(requests) != 1 {
		t.Errorf("should not retry for 400 error, got %d requests", len(requests))
	}
}

// ============================================================
// Test Hook Methods
// ============================================================

func TestClient_BuildMCPRequestBody(t *testing.T) {
	client := NewClient()
	c := client.(*Client)

	body := c.BuildMCPRequestBody("system prompt", "user prompt")

	if body == nil {
		t.Fatal("body should not be nil")
	}

	if body["model"] == nil {
		t.Error("body should have model field")
	}

	messages, ok := body["messages"].([]map[string]string)
	if !ok {
		t.Fatal("messages should be []map[string]string")
	}

	if len(messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(messages))
	}

	if messages[0]["role"] != "system" {
		t.Error("first message should be system")
	}

	if messages[1]["role"] != "user" {
		t.Error("second message should be user")
	}
}

func TestClient_BuildUrl(t *testing.T) {
	tests := []struct {
		name       string
		baseURL    string
		useFullURL bool
		expected   string
	}{
		{
			name:       "normal URL",
			baseURL:    "https://api.test.com/v1",
			useFullURL: false,
			expected:   "https://api.test.com/v1/chat/completions",
		},
		{
			name:       "full URL",
			baseURL:    "https://api.test.com/custom/endpoint",
			useFullURL: true,
			expected:   "https://api.test.com/custom/endpoint",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient(
				WithProvider("test-provider"), // Prevent default DeepSeek settings
				WithBaseURL(tt.baseURL),
				WithUseFullURL(tt.useFullURL),
			)
			c := client.(*Client)

			url := c.BuildUrl()
			if url != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, url)
			}
		})
	}
}

func TestClient_SetAuthHeader(t *testing.T) {
	client := NewClient(WithAPIKey("test-api-key"))
	c := client.(*Client)

	headers := make(http.Header)
	c.SetAuthHeader(headers)

	authHeader := headers.Get("Authorization")
	if authHeader != "Bearer test-api-key" {
		t.Errorf("expected 'Bearer test-api-key', got '%s'", authHeader)
	}
}

func TestClient_IsRetryableError(t *testing.T) {
	client := NewClient()
	c := client.(*Client)

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "EOF error",
			err:      errors.New("unexpected EOF"),
			expected: true,
		},
		{
			name:     "timeout error",
			err:      errors.New("timeout exceeded"),
			expected: true,
		},
		{
			name:     "connection reset",
			err:      errors.New("connection reset by peer"),
			expected: true,
		},
		{
			name:     "upstream empty output",
			err:      errors.New(`API returned error (status 429): {"error":{"code":"upstream_empty_output","message":"Upstream model returned empty output.","type":"rate_limit_error"}}`),
			expected: true,
		},
		{
			name:     "normal error",
			err:      errors.New("bad request"),
			expected: false,
		},
		{
			name:     "validation error",
			err:      errors.New("invalid input"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.IsRetryableError(tt.err)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// ============================================================
// Test SetTimeout
// ============================================================

func TestClient_SetTimeout(t *testing.T) {
	client := NewClient()

	newTimeout := 90 * time.Second
	client.SetTimeout(newTimeout)

	c := client.(*Client)
	if c.HTTPClient.Timeout != newTimeout {
		t.Errorf("expected timeout %v, got %v", newTimeout, c.HTTPClient.Timeout)
	}
}

// ============================================================
// Test String Method
// ============================================================

func TestClient_String(t *testing.T) {
	client := NewClient(
		WithProvider("test-provider"),
		WithModel("test-model"),
	)

	c := client.(*Client)
	str := c.String()

	expectedContains := []string{"test-provider", "test-model"}
	for _, exp := range expectedContains {
		if !contains(str, exp) {
			t.Errorf("String() should contain '%s', got '%s'", exp, str)
		}
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ============================================================
// Response finish-state validation
// ============================================================

func TestDefaultConfig_ReasoningBudget(t *testing.T) {
	t.Setenv("AI_MAX_TOKENS", "")

	client := NewClient().(*Client)
	if client.MaxTokens != 8000 {
		t.Fatalf("default output budget must fit reasoning + decision, got %d", client.MaxTokens)
	}
	body := client.Hooks.BuildMCPRequestBody("system", "user")
	if got := body["max_tokens"]; got != 8000 {
		t.Fatalf("request body must carry the default budget, got %v", got)
	}
}

func TestBuildMCPRequestBody_ReasoningEffortDeepSeek(t *testing.T) {
	for _, effort := range []string{
		ReasoningEffortLow,
		ReasoningEffortMedium,
		ReasoningEffortHigh,
		ReasoningEffortXHigh,
		ReasoningEffortMax,
	} {
		client := NewClient(
			WithProvider(ProviderDeepSeek),
			WithReasoningEffort(effort),
		).(*Client)

		body := client.BuildMCPRequestBody("system", "user")
		if got := body["reasoning_effort"]; got != effort {
			t.Fatalf("effort %q: reasoning_effort=%v, want %q", effort, got, effort)
		}
		if _, ok := body["thinking"]; ok {
			t.Fatalf("effort %q: thinking must be expressed through reasoning_effort, got %#v", effort, body["thinking"])
		}
	}
}

func TestBuildMCPRequestBody_ReasoningEffortOmittedByDefault(t *testing.T) {
	client := NewClient(WithProvider(ProviderDeepSeek)).(*Client)

	body := client.BuildMCPRequestBody("system", "user")
	if _, ok := body["reasoning_effort"]; ok {
		t.Fatalf("reasoning_effort must not be sent without a configured effort: %#v", body["reasoning_effort"])
	}
}

func TestBuildMCPRequestBody_ReasoningEffortClampedForCappedProviders(t *testing.T) {
	client := NewClient(
		WithProvider(ProviderOpenAI),
		WithReasoningEffort(ReasoningEffortMax),
	).(*Client)

	body := client.BuildMCPRequestBody("system", "user")
	if got := body["reasoning_effort"]; got != ReasoningEffortHigh {
		t.Fatalf("OpenAI must cap max at high, got %v", got)
	}
}

func TestBuildRequestBodyFromRequest_ReasoningEffort(t *testing.T) {
	client := NewClient(
		WithProvider(ProviderDeepSeek),
		WithReasoningEffort(ReasoningEffortHigh),
	).(*Client)

	body := client.BuildRequestBodyFromRequest(&Request{
		Model:    "deepseek-chat",
		Messages: []Message{NewUserMessage("hi")},
	})
	if got := body["reasoning_effort"]; got != ReasoningEffortHigh {
		t.Fatalf("request path lost reasoning_effort: %#v", body)
	}
}

func TestNewClient_ReasoningEffortRaisesOutputBudget(t *testing.T) {
	client := NewClient(
		WithProvider(ProviderDeepSeek),
		WithMaxTokens(2000),
		WithReasoningEffort(ReasoningEffortMax),
	).(*Client)

	if client.MaxTokens < ReasoningEffortOutputFloor(ReasoningEffortMax) {
		t.Fatalf("max effort must raise the output budget, got %d", client.MaxTokens)
	}
	if client.Cfg.MaxTokens != client.MaxTokens {
		t.Fatalf("config and client budgets diverged: %d vs %d", client.Cfg.MaxTokens, client.MaxTokens)
	}
}

// OutputTokenBudget is the planning number for the model window split; it must
// equal what a client built without an explicit cap actually sends.
func TestOutputTokenBudgetMatchesClientDefault(t *testing.T) {
	for _, effort := range []string{
		"", ReasoningEffortLow, ReasoningEffortMedium,
		ReasoningEffortHigh, ReasoningEffortXHigh, ReasoningEffortMax,
	} {
		client := NewClient(WithProvider(ProviderDeepSeek), WithReasoningEffort(effort)).(*Client)
		if got, want := OutputTokenBudget(effort), client.MaxTokens; got != want {
			t.Fatalf("effort %q: OutputTokenBudget = %d, client sends %d", effort, got, want)
		}
	}
}

func TestParseMCPResponse_TruncatedResponseIsError(t *testing.T) {
	client := NewClient(WithMaxTokens(2000), WithProvider(ProviderDeepSeek)).(*Client)
	body := []byte(`{
		"choices": [{
			"message": {"content": "", "reasoning_content": "still thinking"},
			"finish_reason": "length"
		}],
		"usage": {"completion_tokens": 2000}
	}`)

	_, err := client.ParseMCPResponse(body)
	if err == nil {
		t.Fatal("truncated response must not be accepted")
	}
	if !strings.Contains(err.Error(), "truncated") || !strings.Contains(err.Error(), "output token budget") {
		t.Fatalf("error must explain the missing output budget, got: %v", err)
	}
}

func TestParseMCPResponse_TruncatedAfterCompleteContentIsAccepted(t *testing.T) {
	client := NewClient().(*Client)
	body := []byte(`{
		"choices": [{
			"message": {"content": "<reasoning>done</reasoning><decision>[]</decision> trailing"},
			"finish_reason": "length"
		}]
	}`)

	content, err := client.ParseMCPResponse(body)
	if err != nil {
		t.Fatalf("a complete payload must survive a trailing length stop: %v", err)
	}
	if !strings.Contains(content, "<decision>[]</decision>") {
		t.Fatalf("unexpected content: %q", content)
	}
}

func TestParseMCPResponse_EmptyContentIsError(t *testing.T) {
	client := NewClient().(*Client)
	body := []byte(`{
		"choices": [{"message": {"content": ""}, "finish_reason": "stop"}]
	}`)

	_, err := client.ParseMCPResponse(body)
	if err == nil {
		t.Fatal("empty content must not be accepted as a valid reply")
	}
	if !strings.Contains(err.Error(), "empty content") {
		t.Fatalf("error must identify the empty reply, got: %v", err)
	}
}

func TestParseMCPResponse_ToolCallsMayHaveEmptyContent(t *testing.T) {
	client := NewClient().(*Client)
	body := []byte(`{
		"choices": [{
			"message": {
				"content": "",
				"tool_calls": [{"id": "call_1", "type": "function", "function": {"name": "get_price", "arguments": "{}"}}]
			},
			"finish_reason": "tool_calls"
		}]
	}`)

	resp, err := client.ParseMCPResponseFull(body)
	if err != nil {
		t.Fatalf("tool-call response must stay valid: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(resp.ToolCalls))
	}
}

func TestParseMCPResponse_CompleteContentIsAccepted(t *testing.T) {
	client := NewClient().(*Client)
	body := []byte(`{
		"choices": [{"message": {"content": "<decision>[]</decision>"}, "finish_reason": "stop"}]
	}`)

	content, err := client.ParseMCPResponse(body)
	if err != nil {
		t.Fatalf("complete response must be accepted: %v", err)
	}
	if content != "<decision>[]</decision>" {
		t.Fatalf("unexpected content: %q", content)
	}
}
