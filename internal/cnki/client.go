// Package cnki 封装中国知网的检索接口.
//
// 设计要点:
//
//   - 知网 kns.cnki.net 全站需要滑块验证, 验证绑定 cookie. 本包不处理验证,
//     只负责在会话有效时发请求, 并在响应中识别出验证跳转后返回 ErrVerifyRequired.
//   - 实测 brief/grid 等检索接口不校验请求签名 (timestamp/nonce/signature 头),
//     因此本包不实现签名. 若知网日后开始强制校验, 需补 getVV 的等价实现.
//   - 结果页返回服务端渲染 HTML, 解析全在 Go 侧完成, 不引入 JS 运行时.
package cnki

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/azazo1/cnki-cli/internal/apperr"
	"github.com/azazo1/cnki-cli/internal/config"
	"github.com/azazo1/cnki-cli/internal/session"
)

// DefaultBaseURL 是知网主站地址.
const DefaultBaseURL = "https://kns.cnki.net"

// DefaultUserAgent 是默认请求标识.
//
// 知网对 UA 不敏感 (实测 iOS/Android/Googlebot 一律同等对待), 这里用一个
// 常见桌面 Chrome 标识即可, 避免出现明显异常的空 UA.
const DefaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
	"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// ErrVerifyRequired 表示知网要求重新做滑块验证, 即当前会话已失效.
//
// 调用方应当据此触发验证接力 (重新接管浏览器) 或提示用户重新登录.
var ErrVerifyRequired = errors.New("知网要求安全验证, 当前会话已失效")

// ErrPermissionDenied 表示接口可用但账号没有相应权限, 常见于全文下载.
var ErrPermissionDenied = errors.New("当前账号没有该资源的访问权限")

// Client 是知网接口客户端.
//
// 同一个 Client 内的请求串行化并强制遵守最小间隔, 避免触发知网限流.
type Client struct {
	baseURL   string
	userAgent string
	interval  time.Duration
	http      *http.Client
	logger    logger
	session   *session.Session

	mu   sync.Mutex
	last time.Time
}

// logger 是 Client 需要的最小日志接口, 便于测试注入.
type logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
}

// Options 描述构造参数.
type Options struct {
	// BaseURL 覆盖知网主站地址, 留空使用 DefaultBaseURL.
	BaseURL string
	// UserAgent 覆盖请求标识.
	UserAgent string
	// Request 提供超时与限速配置.
	Request config.RequestConfig
	// Session 是当前会话, 可以为 nil (此时请求会命中验证跳转).
	Session *session.Session
	// Logger 是日志器.
	Logger logger
}

// NewClient 构造客户端.
func NewClient(opts Options) *Client {
	base := strings.TrimRight(opts.BaseURL, "/")
	if base == "" {
		base = DefaultBaseURL
	}
	ua := opts.UserAgent
	if ua == "" {
		ua = DefaultUserAgent
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 4
	return &Client{
		baseURL:   base,
		userAgent: ua,
		interval:  opts.Request.Interval(),
		http: &http.Client{
			Timeout:   opts.Request.Timeout(),
			Transport: transport,
			// 不自动跟随重定向: 验证跳转需要被识别出来而不是被静默吞掉.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		logger:  opts.Logger,
		session: opts.Session,
	}
}

// BaseURL 返回客户端使用的站点地址.
func (c *Client) BaseURL() string { return c.baseURL }

// SetSession 替换客户端持有的会话.
func (c *Client) SetSession(s *session.Session) { c.session = s }

// Session 返回客户端当前持有的会话.
func (c *Client) Session() *session.Session { return c.session }

// throttled 阻塞到距离上次请求满足最小间隔为止.
func (c *Client) throttled(ctx context.Context) error {
	if c.interval <= 0 {
		return nil
	}
	c.mu.Lock()
	wait := c.interval - time.Since(c.last)
	if wait < 0 {
		wait = 0
	}
	c.last = c.last.Add(c.interval)
	if c.last.Before(time.Now()) {
		c.last = time.Now()
	}
	c.mu.Unlock()

	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Response 是一次原始请求的结果.
type Response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
	// FinalURL 是响应的 Location 头或请求地址, 便于诊断跳转.
	FinalURL string
}

// Text 把响应体按字符串返回.
func (r *Response) Text() string { return string(r.Body) }

// Get 发起 GET 请求.
func (c *Client) Get(ctx context.Context, path string, query url.Values) (*Response, error) {
	target := path
	if len(query) > 0 {
		target = path + "?" + query.Encode()
	}
	return c.do(ctx, http.MethodGet, target, nil, "")
}

// PostForm 发起表单 POST 请求.
func (c *Client) PostForm(ctx context.Context, path string, form url.Values) (*Response, error) {
	return c.do(ctx, http.MethodPost, path, form, FormContentType)
}

// FormContentType 是知网表单接口使用的 Content-Type.
const FormContentType = "application/x-www-form-urlencoded; charset=UTF-8"

// do 是统一的请求执行入口, 负责限速, 请求头, 以及验证跳转识别.
func (c *Client) do(ctx context.Context, method, path string, form url.Values, contentType string) (*Response, error) {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}

	urlStr := path
	if !strings.HasPrefix(path, "http://") && !strings.HasPrefix(path, "https://") {
		urlStr = c.baseURL + path
	}

	req, err := http.NewRequestWithContext(ctx, method, urlStr, body)
	if err != nil {
		return nil, apperr.Wrap(apperr.ExitFailure, err, "构造请求 %s", urlStr)
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Referer", c.baseURL+"/kns8s/")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if ck := c.session.CookieHeader(); ck != "" {
		req.Header.Set("Cookie", ck)
	}

	if err := c.throttled(ctx); err != nil {
		return nil, apperr.Wrap(apperr.ExitInterrupt, err, "请求被中断")
	}

	started := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		if c.logger != nil {
			c.logger.Warn("请求失败", "method", method, "path", path, "err", err)
		}
		return nil, apperr.Wrap(apperr.ExitFailure, err, "请求 %s", urlStr)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, apperr.Wrap(apperr.ExitFailure, err, "读取响应 %s", urlStr)
	}

	out := &Response{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		Body:       data,
		FinalURL:   resp.Header.Get("Location"),
	}
	if c.logger != nil {
		c.logger.Debug("知网请求",
			"method", method,
			"path", path,
			"status", resp.StatusCode,
			"bytes", len(data),
			"elapsed_ms", time.Since(started).Milliseconds())
	}

	if err := detectVerification(out); err != nil {
		return out, err
	}
	if resp.StatusCode >= 400 {
		return out, apperr.Remote("知网返回状态码 %d (%s)", resp.StatusCode, path)
	}
	return out, nil
}

// detectVerification 识别知网的滑块验证跳转与拦截页.
//
// 判定依据来自实测: 未通过验证的请求会被 302 到 /verify/home, 或直接返回
// 一个极短的 "安全验证" 页面.
func detectVerification(resp *Response) error {
	if loc := resp.Header.Get("Location"); strings.Contains(loc, "/verify/") {
		return ErrVerifyRequired
	}
	text := string(resp.Body)
	if len(text) < 4096 {
		if strings.Contains(text, "安全验证") || strings.Contains(text, "captchaType") {
			return ErrVerifyRequired
		}
	}
	return nil
}

// IsVerifyRequired 判断错误是否表示需要重新验证.
func IsVerifyRequired(err error) bool { return errors.Is(err, ErrVerifyRequired) }

// AbsURL 把接口返回的相对地址补全为绝对地址.
func (c *Client) AbsURL(path string) string {
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return c.baseURL + path
}

// Ensure 校验响应可用, 供调用方在解析前统一兜底.
func (r *Response) Ensure() error {
	if r == nil {
		return fmt.Errorf("响应为空")
	}
	return nil
}
