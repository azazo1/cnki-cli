package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/azazo1/cnki-cli/internal/apperr"
	"github.com/azazo1/cnki-cli/internal/cnki"
	"github.com/azazo1/cnki-cli/internal/config"
	"github.com/azazo1/cnki-cli/internal/logging"
	"github.com/azazo1/cnki-cli/internal/session"
)

// Runtime 汇总一次命令执行所需的全部依赖.
//
// 所有子命令都通过它拿到配置, 日志, 会话与知网客户端, 避免各处重复装配,
// 也让测试可以注入替身.
type Runtime struct {
	Config  config.Config
	Logger  *slog.Logger
	Session *session.Session
	Client  *cnki.Client

	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader

	// logLevel 与 logFormat 来自命令行, 覆盖配置文件取值.
	logLevel  string
	logFormat string

	// autoRelogin 允许在会话失效时自动打开浏览器重新验证.
	// 由 --no-relogin 关闭, 供脚本环境使用.
	autoRelogin bool
}

// newRuntime 装配运行时.
func newRuntime(opts Options, logLevel string, logFormat string, autoRelogin bool) (*Runtime, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, apperr.Wrap(apperr.ExitFailure, err, "加载配置")
	}

	level := logLevel
	if level == "" {
		level = os.Getenv(config.EnvLogLevel)
	}
	logger := logging.New(logging.Options{
		Level:  level,
		Format: logFormat,
		Writer: opts.Stderr,
	})

	if base := os.Getenv(config.EnvBaseURL); base != "" {
		cfg.Request.BaseURL = base
	}

	rt := &Runtime{
		Config:      cfg,
		Logger:      logger,
		Stdout:      opts.Stdout,
		Stderr:      opts.Stderr,
		Stdin:       opts.Stdin,
		logLevel:    level,
		logFormat:   logFormat,
		autoRelogin: autoRelogin,
	}
	if err := rt.loadSession(); err != nil {
		return nil, err
	}
	rt.buildClient()
	return rt, nil
}

// loadSession 按优先级加载会话: 环境变量 cookie > 会话文件.
//
// 环境变量优先是为了让 CI 与远程场景能够在不写盘的前提下注入会话.
func (rt *Runtime) loadSession() error {
	if raw := strings.TrimSpace(os.Getenv(config.EnvCookie)); raw != "" {
		cookies, err := session.ParseCookieHeader(raw)
		if err != nil {
			return apperr.Wrap(apperr.ExitUsage, err, "解析环境变量 %s", config.EnvCookie)
		}
		rt.Session = session.New("env:"+config.EnvCookie, cookies)
		rt.Logger.Debug("已从环境变量加载会话", "cookie_count", len(rt.Session.Cookies))
		return nil
	}

	sess, err := session.Load()
	if err != nil {
		return apperr.Wrap(apperr.ExitFailure, err, "加载会话")
	}
	rt.Session = sess
	if sess != nil {
		rt.Logger.Debug("已从文件加载会话", "cookie_count", len(sess.Cookies), "source", sess.Source)
	}
	return nil
}

// buildClient 依据当前会话构造知网客户端.
func (rt *Runtime) buildClient() {
	rt.Client = cnki.NewClient(cnki.Options{
		BaseURL:   rt.Config.Request.BaseURL,
		UserAgent: rt.Config.Request.UserAgent,
		Request:   rt.Config.Request,
		Session:   rt.Session,
		Logger:    rt.Logger,
	})
}

// HasSession 判断当前是否持有可用会话.
func (rt *Runtime) HasSession() bool { return !rt.Session.Empty() }

// SaveSession 把会话落盘并刷新客户端.
func (rt *Runtime) SaveSession(sess *session.Session) error {
	if err := session.Save(sess); err != nil {
		return apperr.Wrap(apperr.ExitFailure, err, "保存会话")
	}
	rt.Session = sess
	rt.Client.SetSession(sess)
	return nil
}

// Login 通过受控浏览器取得会话.
//
// cdpURL 非空时接管该地址上已运行的浏览器, 而不是新开一个.
func (rt *Runtime) Login(ctx context.Context, target string, cdpURL string) (*session.Session, error) {
	if strings.TrimSpace(cdpURL) == "" {
		cdpURL = rt.Config.Browser.CDPURL
	}
	capturer := session.NewBrowserCapturer(session.BrowserOptions{
		ChromePath:    rt.Config.Browser.ChromePath,
		ProfileDir:    config.ChromeProfileDir(),
		Headless:      rt.Config.Browser.Headless,
		VerifyTimeout: rt.Config.Browser.VerifyTimeout(),
		CDPURL:        cdpURL,
		Prompt:        rt.Stderr,
		Logf: func(format string, args ...any) {
			rt.Logger.Debug(fmt.Sprintf(format, args...))
		},
	})
	return capturer.Capture(ctx, target)
}

// EnsureSession 确保会话可用, 必要时按配置策略触发重新验证.
//
// 返回值表示是否发生了重新登录. 调用方在重新登录后应重试原请求.
func (rt *Runtime) EnsureSession(ctx context.Context) (bool, error) {
	if !rt.HasSession() {
		return false, apperr.Session(
			"尚未取得知网会话, 请先执行 cnki auth login")
	}
	return false, nil
}

// HandleExpired 在请求命中验证跳转时按策略处理.
//
// 返回值表示是否已成功恢复会话, 调用方据此决定是否重试.
func (rt *Runtime) HandleExpired(ctx context.Context, err error) (bool, error) {
	if !cnki.IsVerifyRequired(err) {
		return false, err
	}

	switch strings.ToLower(rt.Config.Session.OnExpired) {
	case "fail":
		return false, apperr.Session("知网会话已失效, 且当前配置为不自动重新验证")
	case "prompt":
		fmt.Fprintln(rt.Stderr, "知网会话已失效. 按 Enter 打开浏览器重新验证, 输入 q 放弃:")
		var reply string
		_, _ = fmt.Fscanln(rt.Stdin, &reply)
		if strings.EqualFold(strings.TrimSpace(reply), "q") {
			return false, apperr.Session("用户放弃重新验证")
		}
	}

	if !rt.autoRelogin {
		return false, apperr.Session("知网会话已失效, 且已禁用自动重新验证")
	}

	rt.Logger.Info("知网会话已失效, 开始重新验证")
	sess, err := rt.Login(ctx, "", "")
	if err != nil {
		return false, apperr.Wrap(apperr.ExitSession, err, "重新验证失败")
	}
	if err := rt.SaveSession(sess); err != nil {
		return false, err
	}
	return true, nil
}

// withSessionRetry 执行 fn, 命中会话失效时按策略重登并重试一次.
//
// 只重试一次: 若重新验证后仍然失败, 通常意味着知网侧行为变化,
// 反复重试只会让用户困惑.
func withSessionRetry[T any](ctx context.Context, rt *Runtime, fn func(context.Context) (T, error)) (T, error) {
	result, err := fn(ctx)
	if err == nil {
		return result, nil
	}
	recovered, handleErr := rt.HandleExpired(ctx, err)
	if handleErr != nil {
		var zero T
		return zero, handleErr
	}
	if !recovered {
		var zero T
		return zero, err
	}
	return fn(ctx)
}

// loginRequired 在缺少会话时给出统一的提示.
func (rt *Runtime) loginRequired() error {
	return apperr.Session(
		"尚未取得知网会话. 执行 cnki auth login 打开受控浏览器完成一次验证即可, " +
			"之后所有检索都会直接复用该会话")
}

// closeQuietly 忽略关闭错误, 用于 defer.
func closeQuietly(c io.Closer) {
	if c == nil {
		return
	}
	if err := c.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
		slog.Default().Debug("关闭资源失败", "err", err)
	}
}
