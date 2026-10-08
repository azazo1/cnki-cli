package session

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

// BrowserOptions 描述接管浏览器的行为.
type BrowserOptions struct {
	// ChromePath 是 Chrome 可执行文件路径, 留空自动探测.
	ChromePath string
	// ProfileDir 是独立浏览器 profile 目录.
	//
	// 刻意与用户日常 profile 隔离: 一是避免污染其书签与扩展, 二是日常 profile
	// 可能已被占用而导致启动失败.
	ProfileDir string
	// Headless 控制是否无头启动. 需要人工过滑块时必须为 false.
	Headless bool
	// VerifyTimeout 是等待用户完成验证的超时.
	VerifyTimeout time.Duration
	// CDPURL 指向一个已经在运行且开启了远程调试的浏览器.
	//
	// 指定它时不会启动新浏览器, 而是直接接管该实例. 适合已经在日常浏览器里
	// 登录过知网, 不想再开一个新窗口重新登录的场景. 用法是先以
	// --remote-debugging-port=9222 启动 Chrome, 再运行本程序.
	CDPURL string
	// Prompt 是给用户看的提示输出目标, 为空则不输出.
	Prompt io.Writer
	// Logf 是可选的日志回调.
	Logf func(format string, args ...any)
}

// DefaultCDPPort 是连接已运行浏览器时使用的默认调试端口.
const DefaultCDPPort = 9222

// 知网的滑块验证页路径特征.
const verifyPathMark = "/verify/"

// 验证通过后正常页面必然存在的元素, 用它确认页面真的可用了.
const readyElementSelector = "#classid"

// BrowserCapturer 负责通过受控浏览器取得知网会话.
//
// 工作方式与 npm login 类似: 弹出一个真实浏览器窗口停在知网页面上,
// 用户完成滑块验证或任意形式的登录后, 本程序直接从浏览器读取 cookie 并
// 关闭窗口. 由于用户面对的是真浏览器, 机构 IP 登录, 账号密码, 短信验证码,
// 微信扫码与 CARSI 联邦认证全部天然可用, CLI 无需分别实现各登录协议.
type BrowserCapturer struct {
	opts BrowserOptions
}

// NewBrowserCapturer 构造浏览器接管器.
func NewBrowserCapturer(opts BrowserOptions) *BrowserCapturer {
	if opts.VerifyTimeout <= 0 {
		opts.VerifyTimeout = 5 * time.Minute
	}
	return &BrowserCapturer{opts: opts}
}

// promptf 向用户输出一行提示.
func (c *BrowserCapturer) promptf(format string, args ...any) {
	if c.opts.Prompt == nil {
		return
	}
	fmt.Fprintf(c.opts.Prompt, format+"\n", args...)
}

// logf 输出一行诊断日志.
func (c *BrowserCapturer) logf(format string, args ...any) {
	if c.opts.Logf != nil {
		c.opts.Logf(format, args...)
	}
}

// Capture 打开受控浏览器, 等待用户完成验证, 返回可用的会话.
//
// target 是要访问的知网页面地址, 留空则使用主站检索入口.
func (c *BrowserCapturer) Capture(ctx context.Context, target string) (*Session, error) {
	if target == "" {
		target = "https://kns.cnki.net/kns8s/"
	}

	browser, cleanup, err := c.connect(ctx)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	page, err := browser.Page(proto.TargetCreateTarget{URL: target})
	if err != nil {
		return nil, fmt.Errorf("打开知网页面失败: %w", err)
	}
	defer func() { _ = page.Close() }()

	c.promptf("已在浏览器中打开知网页面.")
	c.promptf("若页面停在安全验证处, 请完成滑块验证; 需要登录个人账号或机构账号时, 也在该页面内完成.")
	c.promptf("完成后无需回到终端, 本程序会自动继续.")

	if err := c.waitUntilReady(ctx, page); err != nil {
		return nil, err
	}

	cookies, err := collectCookies(page)
	if err != nil {
		return nil, err
	}
	if len(cookies) == 0 {
		return nil, errors.New("未从浏览器读取到任何 cnki.net 域下的 cookie")
	}

	c.promptf("验证已通过, 会话已获取.")
	return New("browser", cookies), nil
}

// connect 接管浏览器: 要么连上已运行的实例, 要么启动一个新的.
//
// 返回的 cleanup 负责回收本次接管所创建的资源, 对已运行的实例不做关闭,
// 因为那个浏览器属于用户, 贸然关掉会打断他手头的工作.
func (c *BrowserCapturer) connect(ctx context.Context) (*rod.Browser, func(), error) {
	if url := strings.TrimSpace(c.opts.CDPURL); url != "" {
		return c.connectExisting(url)
	}

	chromePath, err := c.resolveChrome()
	if err != nil {
		return nil, nil, err
	}

	profileDir := c.opts.ProfileDir
	if profileDir == "" {
		profileDir, err = defaultProfileDir()
		if err != nil {
			return nil, nil, err
		}
	}
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		return nil, nil, fmt.Errorf("创建浏览器 profile 目录 %s: %w", profileDir, err)
	}

	l := launcher.New().
		Bin(chromePath).
		UserDataDir(profileDir).
		Headless(c.opts.Headless).
		// 关闭掉会干扰人工操作的自动化提示条.
		Set("disable-infobars").
		Set("no-first-run").
		Set("no-default-browser-check").
		// Chrome 默认把崩溃转储写到用户目录下的固定位置, 该位置在部分受限
		// 环境中不可写, 会导致浏览器启动失败. 这里统一改到本程序自己的
		// profile 目录内, 既避开权限问题也不污染用户目录.
		Set("crash-dumps-dir", filepath.Join(profileDir, "crash")).
		Set("disable-breakpad").
		// Chrome 在受限环境下会因为同名进程检测而误判并退出, 关掉更稳.
		Set("disable-features", "DialMediaRouteProvider,MediaRouter")

	controlURL, err := l.Launch()
	if err != nil {
		return nil, nil, fmt.Errorf(
			"启动浏览器失败: %w\n"+
				"若本机开启了受限沙箱 (例如容器或安全策略), Chrome 在该环境中无法运行. "+
				"可以在宿主环境直接执行本命令, 或用 --cdp 接管一个已经开启远程调试的浏览器", err)
	}

	browser := rod.New().ControlURL(controlURL)
	if err := browser.Connect(); err != nil {
		l.Cleanup()
		return nil, nil, fmt.Errorf("连接浏览器失败: %w", err)
	}
	c.logf("已启动新的受控浏览器, 目标 %s", c.opts.CDPURL)

	cleanup := func() {
		_ = browser.Close()
		l.Cleanup()
	}
	return browser, cleanup, nil
}

// connectExisting 接管一个已经在运行的浏览器实例.
func (c *BrowserCapturer) connectExisting(rawURL string) (*rod.Browser, func(), error) {
	controlURL := NormalizeCDPURL(rawURL)
	if err := probeCDP(controlURL); err != nil {
		hint := "google-chrome"
		if path, err := c.resolveChrome(); err == nil {
			hint = path
		}
		return nil, nil, fmt.Errorf(
			"无法连接到 %s 上的浏览器: %w\n"+
				"请先以调试端口启动 Chrome, 例如:\n"+
				"  \"%s\" --remote-debugging-port=%d",
			controlURL, err, hint, DefaultCDPPort)
	}

	browser := rod.New().ControlURL(controlURL)
	if err := browser.Connect(); err != nil {
		return nil, nil, fmt.Errorf("连接浏览器失败: %w", err)
	}
	c.logf("已接管运行中的浏览器 %s", controlURL)

	// 用户自己的浏览器不由本程序关闭, 否则会打断他正在做的事.
	return browser, func() {}, nil
}

// NormalizeCDPURL 把用户给的调试地址规整成完整的 HTTP 地址.
//
// 接受三种写法: 纯端口号 (9222), 主机加端口 (127.0.0.1:9222), 以及完整
// 地址 (http://127.0.0.1:9222). 纯端口号必须最先判断: 否则补上协议后
// 字符串里会出现冒号, 导致端口补全逻辑被跳过.
func NormalizeCDPURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = strconv.Itoa(DefaultCDPPort)
	}
	if isAllDigits(raw) {
		return "http://127.0.0.1:" + raw
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if parsed.Port() == "" {
		parsed.Host = parsed.Hostname() + ":" + strconv.Itoa(DefaultCDPPort)
	}
	return parsed.String()
}

// isAllDigits 判断字符串是否全为数字.
func isAllDigits(text string) bool {
	if text == "" {
		return false
	}
	for _, r := range text {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// probeCDP 探测调试端口是否可达, 以便给出比连接超时更有用的报错.
func probeCDP(controlURL string) error {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(strings.TrimRight(controlURL, "/") + "/json/version")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("调试端口返回状态码 %d", resp.StatusCode)
	}
	return nil
}

// waitUntilReady 轮询等待用户完成验证.
//
// 判定条件有两个: 地址不再位于验证页, 且正常页面元素已经出现.
// 只判断地址会误判, 因为验证页跳转过程中地址也会短暂变化.
func (c *BrowserCapturer) waitUntilReady(ctx context.Context, page *rod.Page) error {
	deadline := time.Now().Add(c.opts.VerifyTimeout)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	reported := false
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("等待验证被中断: %w", ctx.Err())
		case <-ticker.C:
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("等待验证超时 (%s), 请重新执行并在浏览器窗口内完成验证", c.opts.VerifyTimeout)
		}

		info, err := page.Info()
		if err != nil {
			// 页面正在导航时取信息会失败, 属于正常情况, 继续轮询.
			c.logf("读取页面状态失败, 继续等待: %v", err)
			continue
		}
		if strings.Contains(info.URL, verifyPathMark) {
			if !reported {
				c.promptf("检测到知网安全验证, 请在弹出的浏览器窗口中完成滑块验证...")
				reported = true
			}
			continue
		}

		// 地址已离开验证页, 再用元素确认页面真的渲染完成了.
		has, _, err := page.Has(readyElementSelector)
		if err != nil || !has {
			c.logf("等待页面元素就绪: %v", err)
			continue
		}
		c.logf("验证通过, 当前地址 %s", info.URL)
		return nil
	}
}

// resolveChrome 定位 Chrome 可执行文件.
func (c *BrowserCapturer) resolveChrome() (string, error) {
	if path := strings.TrimSpace(c.opts.ChromePath); path != "" {
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("配置的 Chrome 路径不可用 %s: %w", path, err)
		}
		return path, nil
	}
	if path := strings.TrimSpace(os.Getenv("CNKI_CHROME_PATH")); path != "" {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	if path, found := launcher.LookPath(); found {
		return path, nil
	}
	for _, candidate := range chromeCandidates() {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf(
		"未找到 Chrome 可执行文件, 请通过 config 的 browser.chrome_path 或环境变量 CNKI_CHROME_PATH 指定")
}

// chromeCandidates 列出各平台常见的 Chrome 安装位置.
func chromeCandidates() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		}
	case "windows":
		return []string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		}
	default:
		return []string{
			"/usr/bin/google-chrome",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
		}
	}
}

// defaultProfileDir 返回默认的浏览器 profile 目录.
func defaultProfileDir() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil || cache == "" {
		return "", fmt.Errorf("无法定位缓存目录, 请在配置中显式指定 browser profile 目录: %w", err)
	}
	return filepath.Join(cache, "cnki-cli", "chrome-profile"), nil
}

// collectCookies 读取浏览器中全部 cnki.net 域下的 cookie.
//
// 使用 CDP 的 Network.getAllCookies 而非 document.cookie, 因为后者读不到
// HttpOnly 的会话 cookie. 实测驱动知网检索接口只需要非 HttpOnly 部分,
// 但完整读取能让会话在更多端点上可用, 且便于排查.
func collectCookies(page *rod.Page) ([]Cookie, error) {
	raw, err := proto.NetworkGetAllCookies{}.Call(page)
	if err != nil {
		return nil, fmt.Errorf("读取浏览器 cookie 失败: %w", err)
	}
	cookies := make([]Cookie, 0, len(raw.Cookies))
	for _, item := range raw.Cookies {
		if item == nil {
			continue
		}
		if !strings.Contains(item.Domain, "cnki") {
			continue
		}
		cookie := Cookie{
			Name:     item.Name,
			Value:    item.Value,
			Domain:   item.Domain,
			Path:     item.Path,
			HTTPOnly: item.HTTPOnly,
			Secure:   item.Secure,
		}
		if item.Expires > 0 {
			cookie.Expires = time.Unix(int64(item.Expires), 0)
		}
		cookies = append(cookies, cookie)
	}
	return Normalize(cookies), nil
}
