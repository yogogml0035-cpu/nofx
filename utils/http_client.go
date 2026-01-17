package utils

import (
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// CreateHTTPClient 创建 HTTP 客户端
// useProxy: 是否使用代理
// timeout: 超时时间
func CreateHTTPClient(useProxy bool, timeout time.Duration) *http.Client {
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	if useProxy {
		// 使用系统代理
		transport.Proxy = http.ProxyFromEnvironment
	} else {
		// 不使用代理
		transport.Proxy = nil
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}

// CreateHTTPClientWithSelectiveProxy 创建支持选择性代理的 HTTP 客户端
// noProxyDomains: 不使用代理的域名列表（如 ["api.deepseek.com", "api.openai.com"]）
// timeout: 超时时间
func CreateHTTPClientWithSelectiveProxy(noProxyDomains []string, timeout time.Duration) *http.Client {
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		Proxy: func(req *http.Request) (*url.URL, error) {
			// 检查请求的域名是否在不使用代理的列表中
			host := req.URL.Hostname()
			for _, domain := range noProxyDomains {
				if strings.Contains(host, domain) {
					// 不使用代理
					return nil, nil
				}
			}
			// 使用系统代理
			return http.ProxyFromEnvironment(req)
		},
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}

// GetProxyURL 获取代理 URL（从环境变量）
func GetProxyURL() string {
	proxyURL := os.Getenv("HTTPS_PROXY")
	if proxyURL == "" {
		proxyURL = os.Getenv("HTTP_PROXY")
	}
	if proxyURL == "" {
		proxyURL = os.Getenv("https_proxy")
	}
	if proxyURL == "" {
		proxyURL = os.Getenv("http_proxy")
	}
	return proxyURL
}

// IsProxyConfigured 检查是否配置了代理
func IsProxyConfigured() bool {
	return GetProxyURL() != ""
}
