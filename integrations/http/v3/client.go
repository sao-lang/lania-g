// client.go 实现 http integration 的客户端封装与基础连接/调用能力。
package http

import (
	"encoding/json"
	stdhttp "net/http"
	"time"

	"github.com/go-resty/resty/v2"
)

// Request 定义 HTTP 请求在拦截器和选项中的最小可变操作集合。
type Request interface {
	SetHeader(key, value string) Request
	SetHeaders(headers map[string]string) Request
	SetQueryParam(key, value string) Request
	SetQueryParams(params map[string]string) Request
	SetBody(body interface{}) Request
	SetAuthToken(token string) Request
	SetBasicAuth(username, password string) Request
}

// Response 定义 HTTP 响应在上层使用时暴露的统一读取接口。
type Response interface {
	StatusCode() int
	Status() string
	Header() stdhttp.Header
	Body() []byte
	String() string
	Unmarshal(obj interface{}) error
}

// RequestOption 表示对请求进行补充配置的函数。
type RequestOption func(Request)
// RequestInterceptor 表示请求发送前的拦截处理器。
type RequestInterceptor func(Request) Request
// ResponseInterceptor 表示响应返回后的拦截处理器。
type ResponseInterceptor func(Response) Response
// ErrorInterceptor 表示请求出错时的错误拦截处理器。
type ErrorInterceptor func(error) error

// Config 描述 HTTP client 的初始化配置。
type Config struct {
	Name                 string
	BaseURL              string
	Timeout              time.Duration
	RetryCount           int
	RetryDelay           time.Duration
	RetryDelayMultiplier float64
	DefaultHeaders       map[string]string
	ProxyURL             string

	RequestInterceptors  []RequestInterceptor
	ResponseInterceptors []ResponseInterceptor
	ErrorInterceptors    []ErrorInterceptor
}

// Factory 约定 HTTP client 工厂需要提供的能力。
type Factory interface {
	Default() *Client
	New(cfg Config) (*Client, error)
}

// Client 是基于 resty 封装的 HTTP 客户端。
type Client struct {
	cfg Config
	raw *resty.Client

	requestInterceptors  []RequestInterceptor
	responseInterceptors []ResponseInterceptor
	errorInterceptors    []ErrorInterceptor
}

// DefaultConfig 返回一份可直接使用的默认 HTTP client 配置。
func DefaultConfig() Config {
	return Config{
		Name:                 "default",
		Timeout:              30 * time.Second,
		RetryCount:           3,
		RetryDelay:           time.Second,
		RetryDelayMultiplier: 2.0,
		DefaultHeaders:       map[string]string{},
	}
}

// New 根据配置创建一个 HTTP client。
func New(cfg Config) (*Client, error) {
	cfg = normalizeConfig(cfg)
	raw := resty.New()
	if cfg.BaseURL != "" {
		raw.SetBaseURL(cfg.BaseURL)
	}
	if cfg.Timeout > 0 {
		raw.SetTimeout(cfg.Timeout)
	}
	if cfg.RetryCount > 0 {
		raw.SetRetryCount(cfg.RetryCount)
	}
	if cfg.RetryDelay > 0 {
		raw.SetRetryWaitTime(cfg.RetryDelay)
	}
	if cfg.RetryDelayMultiplier > 0 {
		raw.SetRetryMaxWaitTime(time.Duration(float64(cfg.RetryDelay) * cfg.RetryDelayMultiplier))
	}
	if len(cfg.DefaultHeaders) > 0 {
		raw.SetHeaders(cfg.DefaultHeaders)
	}
	if cfg.ProxyURL != "" {
		raw.SetProxy(cfg.ProxyURL)
	}
	return &Client{
		cfg:                  cfg,
		raw:                  raw,
		requestInterceptors:  append([]RequestInterceptor{}, cfg.RequestInterceptors...),
		responseInterceptors: append([]ResponseInterceptor{}, cfg.ResponseInterceptors...),
		errorInterceptors:    append([]ErrorInterceptor{}, cfg.ErrorInterceptors...),
	}, nil
}

func normalizeConfig(cfg Config) Config {
	def := DefaultConfig()
	if cfg.Name == "" {
		cfg.Name = def.Name
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = def.Timeout
	}
	if cfg.RetryCount <= 0 {
		cfg.RetryCount = def.RetryCount
	}
	if cfg.RetryDelay <= 0 {
		cfg.RetryDelay = def.RetryDelay
	}
	if cfg.RetryDelayMultiplier <= 0 {
		cfg.RetryDelayMultiplier = def.RetryDelayMultiplier
	}
	if cfg.DefaultHeaders == nil {
		cfg.DefaultHeaders = map[string]string{}
	}
	return cfg
}

// Default 返回当前 client 本身，便于满足 Factory 风格接口。
func (c *Client) Default() *Client { return c }

// New 以工厂风格创建一个新的 HTTP client。
func (c *Client) New(cfg Config) (*Client, error) { return New(cfg) }

// Config 返回当前 client 的配置快照。
func (c *Client) Config() Config { return cloneConfig(c.cfg) }

// Raw 返回底层的 resty client，便于做更细粒度的自定义。
func (c *Client) Raw() *resty.Client { return c.raw }

// Get 发起一个 GET 请求。
func (c *Client) Get(url string, opts ...RequestOption) (Response, error) {
	return c.doRequest("GET", url, nil, opts...)
}

// Post 发起一个 POST 请求。
func (c *Client) Post(url string, body interface{}, opts ...RequestOption) (Response, error) {
	return c.doRequest("POST", url, body, opts...)
}

// Put 发起一个 PUT 请求。
func (c *Client) Put(url string, body interface{}, opts ...RequestOption) (Response, error) {
	return c.doRequest("PUT", url, body, opts...)
}

// Patch 发起一个 PATCH 请求。
func (c *Client) Patch(url string, body interface{}, opts ...RequestOption) (Response, error) {
	return c.doRequest("PATCH", url, body, opts...)
}

// Delete 发起一个 DELETE 请求。
func (c *Client) Delete(url string, opts ...RequestOption) (Response, error) {
	return c.doRequest("DELETE", url, nil, opts...)
}

// Request 按给定方法发起一个通用 HTTP 请求。
func (c *Client) Request(method, url string, body interface{}, opts ...RequestOption) (Response, error) {
	return c.doRequest(method, url, body, opts...)
}

// SetBaseURL 设置 client 的基础地址。
func (c *Client) SetBaseURL(url string) { c.raw.SetBaseURL(url); c.cfg.BaseURL = url }
// SetTimeout 设置 client 的请求超时时间。
func (c *Client) SetTimeout(timeout time.Duration) {
	c.raw.SetTimeout(timeout)
	c.cfg.Timeout = timeout
}
// SetHeader 设置一个默认请求头。
func (c *Client) SetHeader(key, value string) { c.raw.SetHeader(key, value) }
// SetHeaders 批量设置默认请求头。
func (c *Client) SetHeaders(headers map[string]string) {
	c.raw.SetHeaders(headers)
}
// SetProxy 设置 client 使用的代理地址。
func (c *Client) SetProxy(proxyURL string) { c.raw.SetProxy(proxyURL); c.cfg.ProxyURL = proxyURL }
// AddRequestInterceptor 追加一个请求拦截器。
func (c *Client) AddRequestInterceptor(interceptor RequestInterceptor) {
	c.requestInterceptors = append(c.requestInterceptors, interceptor)
}
// AddResponseInterceptor 追加一个响应拦截器。
func (c *Client) AddResponseInterceptor(interceptor ResponseInterceptor) {
	c.responseInterceptors = append(c.responseInterceptors, interceptor)
}
// AddErrorInterceptor 追加一个错误拦截器。
func (c *Client) AddErrorInterceptor(interceptor ErrorInterceptor) {
	c.errorInterceptors = append(c.errorInterceptors, interceptor)
}

func (c *Client) doRequest(method, url string, body interface{}, opts ...RequestOption) (Response, error) {
	req := c.raw.R()
	if body != nil {
		req.SetBody(body)
	}
	wrappedReq := &requestWrapper{req: req}
	for _, opt := range opts {
		opt(wrappedReq)
	}
	for _, interceptor := range c.requestInterceptors {
		wrappedReq = interceptor(wrappedReq).(*requestWrapper)
	}
	resp, err := req.Execute(method, url)
	if err != nil {
		for _, interceptor := range c.errorInterceptors {
			err = interceptor(err)
			if err == nil {
				break
			}
		}
		if err != nil {
			return nil, err
		}
	}
	wrappedResp := &responseWrapper{resp: resp}
	for _, interceptor := range c.responseInterceptors {
		wrappedResp = interceptor(wrappedResp).(*responseWrapper)
	}
	return wrappedResp, nil
}

// WithHeader 为单次请求追加一个请求头。
func WithHeader(key, value string) RequestOption {
	return func(req Request) { req.SetHeader(key, value) }
}

// WithHeaders 为单次请求批量追加请求头。
func WithHeaders(headers map[string]string) RequestOption {
	return func(req Request) { req.SetHeaders(headers) }
}

// WithQueryParam 为单次请求追加一个查询参数。
func WithQueryParam(key, value string) RequestOption {
	return func(req Request) { req.SetQueryParam(key, value) }
}

// WithQueryParams 为单次请求批量追加查询参数。
func WithQueryParams(params map[string]string) RequestOption {
	return func(req Request) { req.SetQueryParams(params) }
}

// WithAuthToken 为单次请求设置 Bearer Token。
func WithAuthToken(token string) RequestOption {
	return func(req Request) { req.SetAuthToken(token) }
}

// WithBasicAuth 为单次请求设置 Basic Auth。
func WithBasicAuth(username, password string) RequestOption {
	return func(req Request) { req.SetBasicAuth(username, password) }
}

type requestWrapper struct{ req *resty.Request }

// SetHeader 设置一个请求头（仅影响当前请求）。
func (w *requestWrapper) SetHeader(key, value string) Request { w.req.SetHeader(key, value); return w }

// SetHeaders 批量设置请求头（仅影响当前请求）。
func (w *requestWrapper) SetHeaders(headers map[string]string) Request {
	w.req.SetHeaders(headers)
	return w
}

// SetQueryParam 设置一个查询参数（仅影响当前请求）。
func (w *requestWrapper) SetQueryParam(key, value string) Request {
	w.req.SetQueryParam(key, value)
	return w
}

// SetQueryParams 批量设置查询参数（仅影响当前请求）。
func (w *requestWrapper) SetQueryParams(params map[string]string) Request {
	w.req.SetQueryParams(params)
	return w
}

// SetBody 设置请求体（仅影响当前请求）。
func (w *requestWrapper) SetBody(body interface{}) Request  { w.req.SetBody(body); return w }

// SetAuthToken 设置 Bearer Token（仅影响当前请求）。
func (w *requestWrapper) SetAuthToken(token string) Request { w.req.SetAuthToken(token); return w }

// SetBasicAuth 设置 Basic Auth（仅影响当前请求）。
func (w *requestWrapper) SetBasicAuth(username, password string) Request {
	w.req.SetBasicAuth(username, password)
	return w
}

type responseWrapper struct{ resp *resty.Response }

// StatusCode 返回 HTTP 状态码。
func (w *responseWrapper) StatusCode() int                 { return w.resp.StatusCode() }

// Status 返回 HTTP 状态文本，例如 "200 OK"。
func (w *responseWrapper) Status() string                  { return w.resp.Status() }

// Header 返回响应头。
func (w *responseWrapper) Header() stdhttp.Header          { return w.resp.Header() }

// Body 返回响应体字节数组。
func (w *responseWrapper) Body() []byte                    { return w.resp.Body() }

// String 返回响应体的字符串形式。
func (w *responseWrapper) String() string                  { return w.resp.String() }

// Unmarshal 将响应体按 JSON 反序列化到 obj。
func (w *responseWrapper) Unmarshal(obj interface{}) error { return json.Unmarshal(w.resp.Body(), obj) }

func cloneConfig(cfg Config) Config {
	out := cfg
	if cfg.DefaultHeaders != nil {
		out.DefaultHeaders = make(map[string]string, len(cfg.DefaultHeaders))
		for k, v := range cfg.DefaultHeaders {
			out.DefaultHeaders[k] = v
		}
	}
	out.RequestInterceptors = append([]RequestInterceptor{}, cfg.RequestInterceptors...)
	out.ResponseInterceptors = append([]ResponseInterceptor{}, cfg.ResponseInterceptors...)
	out.ErrorInterceptors = append([]ErrorInterceptor{}, cfg.ErrorInterceptors...)
	return out
}
