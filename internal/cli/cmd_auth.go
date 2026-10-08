package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/azazo1/cnki-cli/internal/apperr"
	"github.com/azazo1/cnki-cli/internal/config"
	"github.com/azazo1/cnki-cli/internal/session"
)

// newAuthCmd 构造认证相关命令.
func (a *app) newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "管理知网会话",
		Long: `管理知网会话.

知网对检索站点实施滑块验证, 且验证绑定在 cookie 上而非 IP 上. 因此需要
先通过一次浏览器交互取得会话, 之后本程序用纯 HTTP 复用该会话, 速度快
且不再需要浏览器.`,
	}
	cmd.AddCommand(
		a.newAuthLoginCmd(),
		a.newAuthStatusCmd(),
		a.newAuthLogoutCmd(),
		a.newAuthCookieCmd(),
	)
	return cmd
}

// newAuthLoginCmd 构造登录命令.
func (a *app) newAuthLoginCmd() *cobra.Command {
	var (
		cookieString string
		cookieFile   string
		fromCurl     string
		target       string
		cdpURL       string
	)
	cmd := &cobra.Command{
		Use:   "login",
		Short: "取得知网会话",
		Long: `取得知网会话.

默认行为与 npm login 类似: 打开一个受控浏览器窗口停在知网页面上, 用户在该
窗口内完成滑块验证或任意形式的登录 (机构 IP, 账号密码, 短信验证码, 微信
扫码, CARSI 联邦认证皆可), 本程序随即读取会话并关闭窗口.

若日常浏览器里已经登录过知网, 可以用 --cdp 直接接管它, 省去重新登录:

  # 先带调试端口启动 Chrome, 然后
  cnki auth login --cdp 9222

在不方便打开浏览器的环境 (远程主机, CI) 中, 可以用三种方式直接注入会话:

  cnki auth login --cookie-string 'SID_kns_new=xxx; Ecp_ClientId=yyy'
  cnki auth login --cookie-file ~/cnki.cookie
  cnki auth login --from-curl ~/request.txt

--from-curl 直接读浏览器 DevTools 里 "Copy as cURL" 的内容, 自动从中提出
Cookie 请求头.`,
		Example: `  cnki auth login
  cnki auth login --cdp 9222
  cnki auth login --cookie-file ~/cnki.cookie`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rt, err := a.requireRuntime()
			if err != nil {
				return err
			}
			ctx := cmd.Context()

			// 手动注入路径优先, 便于脚本与调试使用.
			raw := strings.TrimSpace(cookieString)
			source := "cookie-string"
			switch {
			case raw != "":
			case cookieFile != "":
				data, readErr := os.ReadFile(cookieFile)
				if readErr != nil {
					return apperr.Wrap(apperr.ExitUsage, readErr, "读取 cookie 文件 %s", cookieFile)
				}
				raw = string(data)
				source = "cookie-file:" + cookieFile
			case fromCurl != "":
				data, readErr := os.ReadFile(fromCurl)
				if readErr != nil {
					return apperr.Wrap(apperr.ExitUsage, readErr, "读取 cURL 文件 %s", fromCurl)
				}
				raw = extractCookieFromCurl(string(data))
				source = "curl:" + fromCurl
				if raw == "" {
					return apperr.Usage("未能从 %s 中提取出 Cookie 请求头", fromCurl)
				}
			}

			if raw != "" {
				cookies, parseErr := session.ParseCookieHeader(raw)
				if parseErr != nil {
					return apperr.Wrap(apperr.ExitUsage, parseErr, "解析 cookie")
				}
				sess := session.New(source, cookies)
				if err := rt.SaveSession(sess); err != nil {
					return err
				}
				fmt.Fprintf(a.opts.Stdout, "会话已保存: %s\n", sess.Describe())
				return nil
			}

			sess, err := rt.Login(ctx, target, cdpURL)
			if err != nil {
				return apperr.Wrap(apperr.ExitSession, err, "登录失败")
			}
			if err := rt.SaveSession(sess); err != nil {
				return err
			}
			fmt.Fprintf(a.opts.Stdout, "会话已保存: %s\n", sess.Describe())
			return nil
		},
	}
	cmd.Flags().StringVar(&cookieString, "cookie-string", "", "直接提供的 cookie 字符串")
	cmd.Flags().StringVar(&cookieFile, "cookie-file", "", "从文件读取 cookie")
	cmd.Flags().StringVar(&fromCurl, "from-curl", "", "从 Copy as cURL 的内容中提取 cookie")
	cmd.Flags().StringVar(&target, "target", "", "登录时打开的知网页面地址")
	cmd.Flags().StringVar(&cdpURL, "cdp", "", "接管已开启远程调试的浏览器, 可只给端口号 (默认 9222)")
	return cmd
}

// newAuthStatusCmd 构造会话状态查询命令.
func (a *app) newAuthStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "查看当前会话状态",
		Long: `查看当前会话状态.

默认只读取本地会话文件, 不发起网络请求. 加 --probe 会实际访问一次知网,
用于确认会话是否仍然有效.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rt, err := a.requireRuntime()
			if err != nil {
				return err
			}
			if !rt.HasSession() {
				fmt.Fprintln(a.opts.Stdout, "无会话. 执行 cnki auth login 取得会话.")
				return nil
			}
			fmt.Fprintf(a.opts.Stdout, "%s\n", rt.Session.Describe())
			fmt.Fprintf(a.opts.Stdout, "来源: %s\n", rt.Session.Source)

			if probe, _ := cmd.Flags().GetBool("probe"); probe {
				return probeSession(cmd, rt, a.opts.Stdout)
			}
			return nil
		},
	}
	cmd.Flags().Bool("probe", false, "实际访问一次知网确认会话是否可用")
	return cmd
}

// newAuthLogoutCmd 构造登出命令.
func (a *app) newAuthLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "删除本地保存的会话",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, err := a.requireRuntime(); err != nil {
				return err
			}
			if err := session.Clear(); err != nil {
				return apperr.Wrap(apperr.ExitFailure, err, "删除会话")
			}
			fmt.Fprintln(a.opts.Stdout, "本地会话已删除.")
			return nil
		},
	}
}

// newAuthCookieCmd 构造 cookie 导出命令.
func (a *app) newAuthCookieCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cookie",
		Short: "打印当前会话的 Cookie 请求头",
		Long: `打印当前会话的 Cookie 请求头.

输出可直接用于 curl 或其它工具的 Cookie 头. 注意这是账号访问凭证,
不要写入公开位置.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rt, err := a.requireRuntime()
			if err != nil {
				return err
			}
			if !rt.HasSession() {
				return a.loginRequired()
			}
			fmt.Fprintln(a.opts.Stdout, rt.Session.CookieHeader())
			return nil
		},
	}
}

// probeSession 实际发起一次轻量请求, 验证会话是否仍然可用.
func probeSession(cmd *cobra.Command, rt *Runtime, stdout interface{ Write([]byte) (int, error) }) error {
	ctx := cmd.Context()
	resp, err := rt.Client.Get(ctx, "/kns8s/", nil)
	if err != nil {
		if isVerifyErr(err) {
			fmt.Fprintln(stdout, "会话已失效, 知网要求重新验证.")
			return apperr.Session("会话已失效")
		}
		return err
	}
	if strings.Contains(resp.Text(), "安全验证") {
		fmt.Fprintln(stdout, "会话已失效, 知网要求重新验证.")
		return apperr.Session("会话已失效")
	}
	fmt.Fprintln(stdout, "会话可用.")
	return nil
}

// extractCookieFromCurl 从 cURL 命令文本中提取 Cookie 请求头.
//
// 兼容 -H 'Cookie: ...' 与 --header 'Cookie: ...' 两种写法, 也兼容
// $'...' 形式的转义字符串.
func extractCookieFromCurl(text string) string {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		idx := strings.Index(lower, "cookie:")
		if idx < 0 {
			continue
		}
		value := line[idx+len("cookie:"):]
		// 去掉结尾引号与可能的续行标记.
		value = strings.TrimRight(value, `'\`)
		value = strings.TrimRight(value, `"`)
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

// readLine 从 stdin 读一行, 供交互式确认使用.
func readLine(r interface{ Read([]byte) (int, error) }) (string, error) {
	reader := bufio.NewReader(r)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// sessionPathHint 返回会话文件位置, 供提示信息使用.
func sessionPathHint() string { return config.SessionPath() }
