// Package session 管理知网访问会话.
//
// 知网对 kns.cnki.net 全站实施滑块验证, 该验证绑定在 cookie 上而非 IP 上,
// 因此会话的本质就是一组可复用的 cookie. 实测这组 cookie 一旦取得,
// 纯 net/http 请求即可拿到与浏览器完全一致的结果.
//
// 本包只负责会话的表示与持久化, 浏览器接管见 browser.go.
package session

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// nowFunc 是时间源, 测试中可替换以获得确定性的时间戳.
var nowFunc = time.Now

// Cookie 是持久化用的 cookie 精简表示.
//
// 刻意不保存 HttpOnly 标记: 实测驱动知网检索接口只需要浏览器 document.cookie
// 能读到的那部分, 少存一份凭证就少一份泄漏面.
type Cookie struct {
	Name     string    `json:"name"`
	Value    string    `json:"value"`
	Domain   string    `json:"domain,omitempty"`
	Path     string    `json:"path,omitempty"`
	Expires  time.Time `json:"expires,omitempty"`
	Secure   bool      `json:"secure,omitempty"`
	HTTPOnly bool      `json:"http_only,omitempty"`
}

// Account 是从 Ecp_LoginStuts cookie 中解析出的账号信息.
//
// 为空表示当前会话只是机构 IP 或匿名滑块态, 未登录个人账号.
type Account struct {
	UserName string `json:"user_name,omitempty"`
	ShowName string `json:"show_name,omitempty"`
	UserType string `json:"user_type,omitempty"`
}

// LoggedIn 表示是否存在个人账号信息.
func (a Account) LoggedIn() bool { return a.UserName != "" || a.ShowName != "" }

// Session 是一次可用的知网访问会话.
type Session struct {
	SchemaVersion int       `json:"schema_version"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	Source        string    `json:"source,omitempty"`
	Account       Account   `json:"account,omitempty"`
	Cookies       []Cookie  `json:"cookies"`
}

// New 用一组 cookie 构造会话, 并从中解析账号信息.
func New(source string, cookies []Cookie) *Session {
	now := time.Now()
	s := &Session{
		SchemaVersion: CurrentSchemaVersion,
		CreatedAt:     now,
		UpdatedAt:     now,
		Source:        source,
		Cookies:       Normalize(cookies),
	}
	s.Account = ParseAccount(s)
	return s
}

// Empty 判断会话是否不含任何 cookie.
func (s *Session) Empty() bool { return s == nil || len(s.Cookies) == 0 }

// Value 按键名取 cookie 取值, 不存在时返回空串.
func (s *Session) Value(name string) string {
	if s == nil {
		return ""
	}
	for _, c := range s.Cookies {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

// CookieHeader 把会话渲染成可直接放进 Cookie 请求头的字符串.
func (s *Session) CookieHeader() string {
	if s.Empty() {
		return ""
	}
	parts := make([]string, 0, len(s.Cookies))
	for _, c := range s.Cookies {
		parts = append(parts, c.Name+"="+c.Value)
	}
	return strings.Join(parts, "; ")
}

// HTTPCookies 把会话转换为 net/http 的 cookie 列表.
func (s *Session) HTTPCookies() []*http.Cookie {
	if s.Empty() {
		return nil
	}
	out := make([]*http.Cookie, 0, len(s.Cookies))
	for _, c := range s.Cookies {
		hc := &http.Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Path:     c.Path,
			Domain:   c.Domain,
			Secure:   c.Secure,
			HttpOnly: c.HTTPOnly,
		}
		if !c.Expires.IsZero() {
			hc.Expires = c.Expires
		}
		out = append(out, hc)
	}
	return out
}

// identityCookies 是不会反映会话寿命的客户端标识 cookie.
//
// 它们由页面脚本写入并带有很长的固定有效期 (例如 Ecp_ClientId 是 400 天),
// 与服务端会话是否仍然有效毫无关系. 若不排除, 就会得出"会话还有 400 天"
// 这种明显错误的提示.
var identityCookies = map[string]bool{
	"Ecp_ClientId": true,
	"fingerprint":  true,
	"_wa_gvid":     true,
	"_wa_vid":      true,
	"SID_sug":      true,
	"drlang":       true,
}

// ExpiresHint 返回会话中真正反映寿命的 cookie 的最早过期时间.
//
// 零值表示没有可用的期限信息. 知网的会话主体由服务端维护, SID_kns_new 这类
// 会话 cookie 本身不带过期时间, 因此这个提示只是参考, 判定失效仍以服务端
// 是否跳转验证页为准.
func (s *Session) ExpiresHint() time.Time {
	if s.Empty() {
		return time.Time{}
	}
	var earliest time.Time
	for _, c := range s.Cookies {
		if c.Expires.IsZero() || identityCookies[c.Name] {
			continue
		}
		if earliest.IsZero() || c.Expires.Before(earliest) {
			earliest = c.Expires
		}
	}
	return earliest
}

// Normalize 去重并稳定排序 cookie 列表, 让会话文件可读且便于比对.
func Normalize(cookies []Cookie) []Cookie {
	seen := make(map[string]int, len(cookies))
	out := make([]Cookie, 0, len(cookies))
	for _, c := range cookies {
		if c.Name == "" {
			continue
		}
		if idx, ok := seen[c.Name]; ok {
			out[idx] = c
			continue
		}
		seen[c.Name] = len(out)
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ParseAccount 从会话 cookie 中解析账号信息.
//
// 知网把登录态放在 Ecp_LoginStuts 里, 取值是一个 JSON 字符串,
// 其中的 ShowName 还是 URL 编码过的机构名.
func ParseAccount(s *Session) Account {
	raw := s.Value("Ecp_LoginStuts")
	if raw == "" {
		return Account{}
	}
	var payload struct {
		UserName string `json:"UserName"`
		ShowName string `json:"ShowName"`
		UserType string `json:"UserType"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return Account{}
	}
	show := payload.ShowName
	if decoded, err := urlQueryUnescape(show); err == nil {
		show = decoded
	}
	return Account{UserName: payload.UserName, ShowName: show, UserType: payload.UserType}
}

// ParseCookieHeader 解析形如 "a=1; b=2" 的 Cookie 请求头.
//
// 用于从浏览器 DevTools, curl 命令或用户手工输入中导入会话.
func ParseCookieHeader(raw string) ([]Cookie, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("cookie 内容为空")
	}
	// 容忍整行 "Cookie: a=1; b=2" 的写法.
	if idx := strings.Index(raw, ":"); idx > 0 && !strings.Contains(raw[:idx], "=") {
		raw = raw[idx+1:]
	}
	var out []Cookie
	for _, pair := range strings.Split(raw, ";") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		name, value, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		out = append(out, Cookie{
			Name:   name,
			Value:  strings.TrimSpace(value),
			Domain: ".cnki.net",
			Path:   "/",
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("未能从内容中解析出任何 cookie")
	}
	return out, nil
}

// Describe 输出一行会话摘要, 供 CLI 展示.
func (s *Session) Describe() string {
	if s.Empty() {
		return "无会话"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "cookie %d 项", len(s.Cookies))
	if s.Account.LoggedIn() {
		fmt.Fprintf(&b, ", 账号 %s", s.Account.UserName)
		if s.Account.ShowName != "" {
			fmt.Fprintf(&b, " (%s)", s.Account.ShowName)
		}
	} else {
		b.WriteString(", 未登录个人账号")
	}
	if hint := s.ExpiresHint(); !hint.IsZero() {
		fmt.Fprintf(&b, ", cookie 期限约 %s", humanDuration(time.Until(hint)))
	}
	return b.String()
}

// humanDuration 把时长渲染成便于阅读的中文表述.
//
// 会话期限跨度量级很大 (分钟到数月), 因此按量级选单位, 避免出现
// "9599h59m0s" 这种难以判读的形式.
func humanDuration(d time.Duration) string {
	if d <= 0 {
		return "已过期"
	}
	switch {
	case d >= 48*time.Hour:
		return fmt.Sprintf("%d 天", int(d.Hours()/24))
	case d >= time.Hour:
		return fmt.Sprintf("%d 小时", int(d.Hours()))
	case d >= time.Minute:
		return fmt.Sprintf("%d 分钟", int(d.Minutes()))
	default:
		return fmt.Sprintf("%d 秒", int(d.Seconds()))
	}
}
