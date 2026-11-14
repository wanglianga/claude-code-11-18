package fetcher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"
)

// FetchResult 保留原始正文与访问结果；404 等永久失败由调用方记为不可访问。
type FetchResult struct {
	URL         string
	StatusCode  int
	Body        string
	Fingerprint string
	Attempts    int
	Accessible  bool
	Error       string
}

type sourceLimit struct {
	mu        sync.Mutex
	lastAt    time.Time
	minGap    time.Duration
	sem       chan struct{}
}

// Client 按来源隔离间隔与并发限制；手动补抓与自动采集走同一限流通道，绕不开限额。
type Client struct {
	httpClient *http.Client
	mu         sync.Mutex
	limits     map[int64]*sourceLimit
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 20 * time.Second},
		limits:     map[int64]*sourceLimit{},
	}
}

func (c *Client) limiter(sourceID int64, intervalMs, maxConcurrency int) *sourceLimit {
	c.mu.Lock()
	defer c.mu.Unlock()
	l, ok := c.limits[sourceID]
	if !ok {
		if maxConcurrency <= 0 {
			maxConcurrency = 1
		}
		l = &sourceLimit{minGap: time.Duration(intervalMs) * time.Millisecond, sem: make(chan struct{}, maxConcurrency)}
		c.limits[sourceID] = l
	}
	return l
}

// Fetch 遵守来源间隔与并发限制；对拒绝访问（403/429/5xx）与网络错误有限次重试。
// 404/410 不重试，直接以 Accessible=false 返回，由业务记为“不可访问”而非撤标。
func (c *Client) Fetch(ctx context.Context, sourceID int64, intervalMs, maxConcurrency, maxRetries int, url string) *FetchResult {
	l := c.limiter(sourceID, intervalMs, maxConcurrency)
	res := &FetchResult{URL: url}
	if maxRetries < 0 {
		maxRetries = 0
	}
	for attempt := 1; attempt <= maxRetries+1; attempt++ {
		res.Attempts = attempt
		// 间隔控制
		l.mu.Lock()
		wait := time.Until(l.lastAt.Add(l.minGap))
		l.mu.Unlock()
		if wait > 0 {
			select {
			case <-ctx.Done():
				res.Error = ctx.Err().Error()
				return res
			case <-time.After(wait):
			}
		}
		// 并发控制
		select {
		case l.sem <- struct{}{}:
		case <-ctx.Done():
			res.Error = ctx.Err().Error()
			return res
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			<-l.sem
			res.Error = err.Error()
			return res
		}
		resp, err := c.httpClient.Do(req)
		l.mu.Lock()
		l.lastAt = time.Now()
		l.mu.Unlock()
		<-l.sem
		if err != nil {
			res.Error = err.Error()
			if !retryableNetErr(err) || attempt == maxRetries+1 {
				return res
			}
			backoff(ctx, attempt)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		res.StatusCode = resp.StatusCode
		sum := sha256.Sum256(normalize(body))
		res.Fingerprint = hex.EncodeToString(sum[:])
		switch {
		case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
			res.Accessible = false
			res.Error = "http " + resp.Status
			return res // 失效链接：记录不可访问，不重试、不推断撤标
		case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			res.Error = "http " + resp.Status
			if attempt == maxRetries+1 {
				res.Accessible = false
				return res
			}
			backoff(ctx, attempt)
			continue
		case resp.StatusCode >= 400:
			res.Accessible = false
			res.Error = "http " + resp.Status
			return res
		default:
			res.Body = string(body)
			res.Accessible = true
			res.Error = ""
			return res
		}
	}
	return res
}

func retryableNetErr(err error) bool {
	return !errors.Is(err, context.Canceled)
}

func backoff(ctx context.Context, attempt int) {
	d := time.Duration(200*attempt) * time.Millisecond
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

// normalize 统一换行，保证同内容页面指纹稳定。
func normalize(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for _, ch := range b {
		if ch == '\r' {
			continue
		}
		out = append(out, ch)
	}
	return out
}
