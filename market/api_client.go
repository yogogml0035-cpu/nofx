package market

import (
	"log"
	"net/http"
	"nofx/hook"
	"time"
)

// APIClient 行情 HTTP 客户端（Gate 公开行情），供 market 包内部各数据源共用。
type APIClient struct {
	client *http.Client
}

func NewAPIClient() *APIClient {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	hookRes := hook.HookExec[hook.SetHttpClientResult](hook.SET_HTTP_CLIENT, client)
	if hookRes != nil && hookRes.Error() == nil {
		log.Printf("Using HTTP client set by Hook")
		client = hookRes.GetResult()
	}

	return &APIClient{
		client: client,
	}
}
