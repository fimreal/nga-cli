package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"nga/internal/cache"
	"nga/internal/filter"
	"nga/internal/limiter"
	"nga/internal/parser"
	"nga/pkg/model"
)

const (
	DefaultBaseURL   = "https://ngabbs.com"
	DefaultUserAgent = "NGA_skull/90412(iPhone; iOS 17.2; Scale/3.00)"
	DefaultTimeout   = 15 * time.Second
)

// Config holds client operational settings.
type Config struct {
	BaseURL     string
	UserAgent   string
	ProxyURL    string
	CookieUID   int64
	CookieCID   string
	RateLimitMs int
	Timeout     time.Duration
	CacheStore  *cache.Store
	NoCache     bool
}

// Client interacts with NGA endpoints.
type Client struct {
	config      Config
	httpClient  *http.Client
	limiter     *limiter.RateLimiter
	cache       *cache.Store
	mu          sync.Mutex
	recentPosts map[string]time.Time
}

// New creates an initialized NGA Client.
func New(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.UserAgent == "" {
		cfg.UserAgent = DefaultUserAgent
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	interval := 1500 * time.Millisecond
	if cfg.RateLimitMs > 0 {
		interval = time.Duration(cfg.RateLimitMs) * time.Millisecond
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	if cfg.ProxyURL != "" {
		pURL, err := url.Parse(cfg.ProxyURL)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy URL %q: %w", cfg.ProxyURL, err)
		}
		transport.Proxy = http.ProxyURL(pURL)
	}

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   cfg.Timeout,
	}

	return &Client{
		config:      cfg,
		httpClient:  httpClient,
		limiter:     limiter.New(interval, 400*time.Millisecond),
		cache:       cfg.CacheStore,
		recentPosts: make(map[string]time.Time),
	}, nil
}

// FetchThreadPage fetches a single page of a thread by TID with SQLite cache support.
func (c *Client) FetchThreadPage(ctx context.Context, tid int64, page int, bypassCache bool) (*model.ThreadDetail, error) {
	if page <= 0 {
		page = 1
	}

	// 1. Check local cache
	if !bypassCache && !c.config.NoCache && c.cache != nil {
		if cached, hit := c.cache.GetPage(tid, page, false); hit {
			return cached, nil
		}
	}

	// 2. Fetch from network
	reqURL := fmt.Sprintf("%s/read.php?tid=%d&page=%d&lite=js", c.config.BaseURL, tid, page)

	body, err := c.doGet(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	// NGA lite=js is typically encoded in GBK/GB18030
	utf8Bytes, err := DecodeGBK(body)
	if err != nil {
		// Fallback to raw body if decoding fails
		utf8Bytes = body
	}

	thread, err := parser.ParseLiteJS(utf8Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse thread response: %w", err)
	}

	// 3. Save to local cache
	if !c.config.NoCache && c.cache != nil {
		_ = c.cache.SavePage(thread, page)
	}

	return thread, nil
}

// FetchThreadTail fetches the last N floors of a thread with minimal requests.
func (c *Client) FetchThreadTail(ctx context.Context, tid int64, n int) (*model.ThreadDetail, error) {
	if n <= 0 {
		n = 10
	}

	// 1. Fetch Page 1 (bypass cache to get latest totalPosts metadata)
	p1, err := c.FetchThreadPage(ctx, tid, 1, true)
	if err != nil {
		return nil, err
	}

	totalPosts := p1.TotalPosts
	if totalPosts <= 0 {
		totalPosts = len(p1.Posts)
	}

	// If total posts is within page 1, slice and return immediately (1 request total)
	if p1.TotalPages <= 1 || totalPosts <= 20 {
		if len(p1.Posts) > n {
			p1.Posts = p1.Posts[len(p1.Posts)-n:]
		}
		return p1, nil
	}

	// Calculate target start floor and page range
	startFloor := totalPosts - n
	if startFloor < 0 {
		startFloor = 0
	}

	rowsPerPage := 20
	startPage := (startFloor / rowsPerPage) + 1
	endPage := p1.TotalPages
	if startPage > endPage {
		startPage = endPage
	}

	var allPosts []model.Post

	// If startPage is 1, keep p1 posts
	if startPage == 1 {
		allPosts = append(allPosts, p1.Posts...)
	}

	// Fetch required tail pages (historical pages use cache, endPage bypasses cache)
	for page := startPage; page <= endPage; page++ {
		if page == 1 {
			continue // Already fetched
		}
		bypass := (page == endPage)
		p, err := c.FetchThreadPage(ctx, tid, page, bypass)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch tail page %d: %w", page, err)
		}
		allPosts = append(allPosts, p.Posts...)
	}

	// Deduplicate by floor and sort
	seen := make(map[int]bool)
	var uniquePosts []model.Post
	for _, p := range allPosts {
		if !seen[p.Floor] {
			seen[p.Floor] = true
			uniquePosts = append(uniquePosts, p)
		}
	}

	// Take the last n posts
	if len(uniquePosts) > n {
		uniquePosts = uniquePosts[len(uniquePosts)-n:]
	}

	p1.Posts = uniquePosts
	p1.CurrentPage = endPage
	return p1, nil
}

// FollowOptions defines the streaming tail parameters.
type FollowOptions struct {
	Tid           int64
	InitialLines  int
	BaseInterval  time.Duration
	MaxInterval   time.Duration
	IdleThreshold int
	FilterOpts    filter.Options
	OnPost        func(post model.Post)
	OnError       func(err error)
}

// FollowThread streams newly added posts in real-time with jittered backoff.
func (c *Client) FollowThread(ctx context.Context, opts FollowOptions) error {
	if opts.BaseInterval <= 0 {
		opts.BaseInterval = 3 * time.Second
	}
	if opts.MaxInterval <= opts.BaseInterval {
		opts.MaxInterval = 10 * time.Second
	}
	if opts.IdleThreshold <= 0 {
		opts.IdleThreshold = 5
	}
	if opts.InitialLines <= 0 {
		opts.InitialLines = 10
	}

	// 1. Initial fetch
	initial, err := c.FetchThreadTail(ctx, opts.Tid, opts.InitialLines)
	if err != nil {
		return fmt.Errorf("initial tail fetch failed: %w", err)
	}

	highestFloor := -1
	for _, p := range initial.Posts {
		if p.Floor > highestFloor {
			highestFloor = p.Floor
		}
	}

	// Apply filter to initial posts and emit
	filteredInitial := filter.Apply(initial, opts.FilterOpts)
	for _, p := range filteredInitial.Posts {
		if opts.OnPost != nil {
			opts.OnPost(p)
		}
	}

	currentInterval := opts.BaseInterval
	idleCount := 0
	totalPages := initial.TotalPages
	if totalPages <= 0 {
		totalPages = 1
	}

	// 2. Continuous polling loop
	for {
		// Calculate jitter (±500ms)
		jitterMs := (time.Now().UnixNano()%1000 - 500) * int64(time.Millisecond)
		sleepDuration := currentInterval + time.Duration(jitterMs)
		if sleepDuration < 2*time.Second {
			sleepDuration = 2 * time.Second
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(sleepDuration):
		}

		// Fetch the latest active page
		latest, err := c.FetchThreadPage(ctx, opts.Tid, totalPages, true)
		if err != nil {
			if opts.OnError != nil {
				opts.OnError(err)
			}
			// On error, back off to max interval to protect IP
			currentInterval = opts.MaxInterval
			continue
		}

		// Check if a new page was created
		if latest.TotalPages > totalPages {
			totalPages = latest.TotalPages
			newPage, err := c.FetchThreadPage(ctx, opts.Tid, totalPages, true)
			if err == nil {
				latest = newPage
			}
		}

		// Collect new posts whose floor > highestFloor
		var newPosts []model.Post
		for _, p := range latest.Posts {
			if p.Floor > highestFloor {
				highestFloor = p.Floor
				newPosts = append(newPosts, p)
			}
		}

		if len(newPosts) > 0 {
			idleCount = 0
			currentInterval = opts.BaseInterval // Reset interval on activity

			tempThread := &model.ThreadDetail{
				Tid:      opts.Tid,
				AuthorID: latest.AuthorID,
				Posts:    newPosts,
			}
			filteredNew := filter.Apply(tempThread, opts.FilterOpts)
			for _, p := range filteredNew.Posts {
				if opts.OnPost != nil {
					opts.OnPost(p)
				}
			}
		} else {
			idleCount++
			if idleCount >= opts.IdleThreshold {
				// Exponential backoff up to maxInterval
				nextInterval := currentInterval + 2*time.Second
				if nextInterval > opts.MaxInterval {
					nextInterval = opts.MaxInterval
				}
				currentInterval = nextInterval
			}
		}
	}
}

// DoctorProbe performs connectivity and anti-risk diagnostics.
func (c *Client) DoctorProbe(ctx context.Context, testTid int64) (*model.DoctorReport, error) {
	if testTid <= 0 {
		testTid = 38888888
	}

	report := &model.DoctorReport{
		Timestamp: time.Now().Format(time.RFC3339),
		Proxy:     c.config.ProxyURL,
	}

	// 1. Check DNS resolution for key NGA domains
	domains := []string{"ngabbs.com", "bbs.nga.cn"}
	for _, domain := range domains {
		start := time.Now()
		ips, err := net.DefaultResolver.LookupHost(ctx, domain)
		latency := time.Since(start).Milliseconds()
		check := model.EndpointCheck{
			Domain:    domain,
			IPs:       ips,
			LatencyMs: latency,
		}
		if err != nil {
			check.Error = err.Error()
		}
		report.Endpoints = append(report.Endpoints, check)
	}

	// 2. Probe HTTP endpoint with configured UA & Proxy
	probeURL := fmt.Sprintf("%s/read.php?tid=%d&lite=js", c.config.BaseURL, testTid)
	probeStart := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL, nil)
	if err != nil {
		report.Message = fmt.Sprintf("Failed to construct probe request: %v", err)
		return report, nil
	}
	c.applyHeaders(req)

	resp, err := c.httpClient.Do(req)
	report.ProbeLatencyMs = time.Since(probeStart).Milliseconds()
	if err != nil {
		report.Message = fmt.Sprintf("Probe HTTP request failed: %v", err)
		return report, nil
	}
	defer resp.Body.Close()

	report.ProbeHTTPCode = resp.StatusCode
	body, _ := io.ReadAll(resp.Body)
	decoded, _ := DecodeGBK(body)
	bodyText := string(decoded)

	if resp.StatusCode == http.StatusOK {
		if strings.Contains(bodyText, "window.script_muti_get_var_store") {
			report.ProbeSuccess = true
			report.Message = "Connectivity OK. Mobile UA bypassed WAF successfully; JSON payload received."
		} else if strings.Contains(bodyText, "ERROR:15") || strings.Contains(bodyText, "访客不能直接访问") {
			report.WAFChallenged = true
			report.Message = "Encountered NGA WAF (ERROR:15 JS Challenge). Try updating User-Agent or using high-reputation proxy."
		} else {
			report.Message = fmt.Sprintf("Received 200 OK but response is unexpected: %s", truncate(bodyText, 100))
		}
	} else if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		report.WAFChallenged = true
		report.Message = fmt.Sprintf("WAF/Rate limit triggered (HTTP %d). Cooldown or proxy switch required.", resp.StatusCode)
	} else {
		report.Message = fmt.Sprintf("Unexpected HTTP status %d", resp.StatusCode)
	}

	return report, nil
}

func (c *Client) doGet(ctx context.Context, reqURL string) ([]byte, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	c.applyHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		c.limiter.RecordBlocked()
		return nil, fmt.Errorf("NGA access blocked (HTTP %d). Rate limit or WAF triggered", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("NGA unexpected HTTP status %d", resp.StatusCode)
	}

	c.limiter.RecordSuccess()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return body, nil
}

func (c *Client) applyHeaders(req *http.Request) {
	req.Header.Set("User-Agent", c.config.UserAgent)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	req.Header.Set("Connection", "keep-alive")

	if c.config.CookieUID > 0 && c.config.CookieCID != "" {
		cookieStr := fmt.Sprintf("ngaPassportUid=%d; ngaPassportCid=%s", c.config.CookieUID, c.config.CookieCID)
		req.Header.Set("Cookie", cookieStr)
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}


// ReplyOptions holds parameters for replying to an NGA thread.
type ReplyOptions struct {
	Tid       int64
	Fid       int64
	Content   string
	QuotePost *model.Post
	Force     bool
}

// NewThreadOptions holds parameters for publishing a new thread.
type NewThreadOptions struct {
	Fid     int64
	Subject string
	Content string
	Force   bool
}

// PostResult represents the parsed outcome of a reply or post action.
type PostResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Tid     int64  `json:"tid,omitempty"`
	Pid     int64  `json:"pid,omitempty"`
}

// PostReply submits a reply to an NGA thread with anti-duplicate protection and GBK encoding.
func (c *Client) PostReply(ctx context.Context, opts ReplyOptions) (*PostResult, error) {
	if c.config.CookieUID <= 0 || c.config.CookieCID == "" {
		return nil, fmt.Errorf("发帖未鉴权：未检测到有效 Cookie UID/CID，请使用 'nga profile set' 或 'nga config set' 配置凭据")
	}

	content := strings.TrimSpace(opts.Content)
	if content == "" {
		return nil, fmt.Errorf("回复内容不能为空")
	}

	// 1. Anti-duplicate submission check
	dupKey := fmt.Sprintf("reply:%d:%s", opts.Tid, content)
	if err := c.checkDuplicate(dupKey, opts.Force); err != nil {
		return nil, err
	}

	// 2. Assemble quote if specified
	if opts.QuotePost != nil {
		quoteBBCode := fmt.Sprintf(
			"[quote][pid=%d,%d,%d]Reply[/pid] [b]Post by [uid=%d]%s[/uid] (%s):[/b]\n%s[/quote]\n\n",
			opts.QuotePost.Pid,
			opts.QuotePost.Tid,
			opts.QuotePost.Floor,
			opts.QuotePost.AuthorID,
			opts.QuotePost.Author,
			opts.QuotePost.PostDate,
			opts.QuotePost.Content,
		)
		content = quoteBBCode + content
	}

	// 3. Encode content to GBK
	gbkContentBytes, err := EncodeGBK(content)
	if err != nil {
		return nil, fmt.Errorf("正文 GBK 编码失败: %w", err)
	}

	formBody := fmt.Sprintf(
		"action=reply&step=2&tid=%d&post_content=%s",
		opts.Tid,
		url.QueryEscape(string(gbkContentBytes)),
	)
	if opts.Fid > 0 {
		formBody += fmt.Sprintf("&fid=%d", opts.Fid)
	}

	reqURL := fmt.Sprintf("%s/post.php?lite=js", c.config.BaseURL)
	respBytes, err := c.doPostForm(ctx, reqURL, []byte(formBody))
	if err != nil {
		return nil, err
	}

	return c.parsePostResponse(respBytes)
}

// PostNewThread publishes a new thread into a forum (FID).
func (c *Client) PostNewThread(ctx context.Context, opts NewThreadOptions) (*PostResult, error) {
	if c.config.CookieUID <= 0 || c.config.CookieCID == "" {
		return nil, fmt.Errorf("发帖未鉴权：未检测到有效 Cookie UID/CID，请使用 'nga profile set' 或 'nga config set' 配置凭据")
	}

	subject := strings.TrimSpace(opts.Subject)
	if subject == "" {
		return nil, fmt.Errorf("主贴标题不能为空")
	}
	content := strings.TrimSpace(opts.Content)
	if content == "" {
		return nil, fmt.Errorf("主贴内容不能为空")
	}

	dupKey := fmt.Sprintf("thread:%d:%s:%s", opts.Fid, subject, content)
	if err := c.checkDuplicate(dupKey, opts.Force); err != nil {
		return nil, err
	}

	gbkSubjectBytes, err := EncodeGBK(subject)
	if err != nil {
		return nil, fmt.Errorf("标题 GBK 编码失败: %w", err)
	}
	gbkContentBytes, err := EncodeGBK(content)
	if err != nil {
		return nil, fmt.Errorf("正文 GBK 编码失败: %w", err)
	}

	formBody := fmt.Sprintf(
		"action=new&step=2&fid=%d&post_subject=%s&post_content=%s",
		opts.Fid,
		url.QueryEscape(string(gbkSubjectBytes)),
		url.QueryEscape(string(gbkContentBytes)),
	)

	reqURL := fmt.Sprintf("%s/post.php?lite=js", c.config.BaseURL)
	respBytes, err := c.doPostForm(ctx, reqURL, []byte(formBody))
	if err != nil {
		return nil, err
	}

	return c.parsePostResponse(respBytes)
}

func (c *Client) checkDuplicate(key string, force bool) error {
	if force {
		return nil
	}

	hashBytes := sha256.Sum256([]byte(key))
	hashKey := hex.EncodeToString(hashBytes[:])

	// If persistent SQLite cache is enabled, check cross-process persistent submissions
	if c.cache != nil {
		return c.cache.CheckAndRecordSubmission(hashKey, 30*time.Second, force)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if lastTime, exists := c.recentPosts[hashKey]; exists {
		if time.Since(lastTime) < 30*time.Second {
			remain := int((30*time.Second - time.Since(lastTime)).Seconds())
			if remain <= 0 {
				remain = 1
			}
			return fmt.Errorf("防重机制触发：检测到 30 秒内向相同目标重复提交相同内容（冷却剩余 %d 秒）。如确需重发请使用 --force 参数", remain)
		}
	}

	c.recentPosts[hashKey] = time.Now()
	return nil
}

func (c *Client) doPostForm(ctx context.Context, reqURL string, formBytes []byte) ([]byte, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(formBytes))
	if err != nil {
		return nil, err
	}
	c.applyHeaders(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP post request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		c.limiter.RecordBlocked()
		return nil, fmt.Errorf("NGA access blocked (HTTP %d). Rate limit or WAF triggered", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("NGA unexpected HTTP status %d", resp.StatusCode)
	}

	c.limiter.RecordSuccess()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return body, nil
}

func (c *Client) parsePostResponse(body []byte) (*PostResult, error) {
	var utf8Bytes []byte
	if utf8.Valid(body) && (bytes.Contains(body, []byte("成功")) || bytes.Contains(body, []byte("error"))) {
		utf8Bytes = body
	} else if decoded, err := DecodeGBK(body); err == nil {
		utf8Bytes = decoded
	} else {
		utf8Bytes = body
	}
	bodyText := string(utf8Bytes)

	// Clean JSON boundaries
	start := bytes.IndexByte(utf8Bytes, '{')
	end := bytes.LastIndexByte(utf8Bytes, '}')
	if start != -1 && end != -1 && end > start {
		var raw map[string]interface{}
		if err := json.Unmarshal(utf8Bytes[start:end+1], &raw); err == nil {
			if errMsg, ok := raw["error"]; ok && errMsg != nil {
				return nil, fmt.Errorf("发帖失败: %v", errMsg)
			}
			if data, ok := raw["data"].(map[string]interface{}); ok {
				if msgMap, ok := data["__MESSAGE"].(map[string]interface{}); ok {
					var msgs []string
					for _, v := range msgMap {
						msgs = append(msgs, fmt.Sprint(v))
					}
					msg := strings.Join(msgs, " ")
					if strings.Contains(msg, "成功") {
						return &PostResult{
							Success: true,
							Message: msg,
						}, nil
					}
					return nil, fmt.Errorf("发帖提示: %s", msg)
				}
			}
		}
	}

	// Fallback text check
	if strings.Contains(bodyText, "发帖成功") || strings.Contains(bodyText, "操作成功") || strings.Contains(bodyText, "回复成功") {
		return &PostResult{
			Success: true,
			Message: "发帖/回复成功",
		}, nil
	}

	return nil, fmt.Errorf("发帖返回异常: %s", truncate(bodyText, 120))
}
