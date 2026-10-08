// Package cli 实现 cnki 命令行的命令树.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/azazo1/cnki-cli/internal/apperr"
	"github.com/azazo1/cnki-cli/internal/output"
)

// Options 是命令行入口参数.
type Options struct {
	Args    []string
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
	Version string
}

// app 持有一次命令执行的全局状态.
type app struct {
	opts Options
	rt   *Runtime

	logLevel    string
	logFormat   string
	outputFmt   string
	jsonOut     bool
	noRelogin   bool
	delayMillis int
}

// Execute 解析参数并执行命令, 返回进程退出码.
//
// 所有错误都在这里统一转为退出码与一行提示, 子命令只需返回 error.
func Execute(opts Options) int {
	if opts.Stdin == nil {
		opts.Stdin = os.Stdin
	}
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}

	a := &app{opts: opts}
	root := a.newRootCmd()
	root.SetArgs(opts.Args)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err := root.ExecuteContext(ctx)
	if err != nil {
		var usageErr *apperr.Error
		// 用法错误才提示如何求助, 其余错误直接给结论, 避免噪音.
		if errors.As(err, &usageErr) && usageErr.Code == apperr.ExitUsage {
			fmt.Fprintf(opts.Stderr, "错误: %s\n", err)
			fmt.Fprintf(opts.Stderr, "用 cnki --help 或 cnki <命令> --help 查看用法.\n")
		} else {
			fmt.Fprintf(opts.Stderr, "错误: %s\n", err)
		}
		return int(apperr.ExitCodeOf(err))
	}
	return int(apperr.ExitOK)
}

// newRootCmd 构造根命令.
func (a *app) newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "cnki",
		Short: "中国知网命令行工具",
		Long: `中国知网命令行工具.

支持知网的全部检索入口: 一框式检索, 高级检索, 专业检索, 句子检索,
作者发文检索, 以及学科分组与题录导出.

首次使用需要执行一次登录:

  cnki auth login

该命令会打开一个受控浏览器窗口, 在其中完成知网的安全验证后, 本程序会
读取会话并保存, 之后的检索都会直接复用该会话, 不再需要浏览器.

输出默认是便于阅读的表格, 加 --json 可得到结构化数据以便管道处理.
日志只写 stderr, 因此 --json 的输出可以直接交给 jq.`,
		Version:       a.opts.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			// 帮助与版本查询不需要装配网络相关依赖.
			if cmd.Name() == "help" || cmd.Name() == "version" {
				return nil
			}
			rt, err := newRuntime(a.opts, a.logLevel, a.logFormat, !a.noRelogin)
			if err != nil {
				return err
			}
			rt.logLevel = a.logLevel
			rt.logFormat = a.logFormat
			// 命令行的 --delay 覆盖配置文件里的请求间隔.
			if a.delayMillis > 0 {
				rt.Config.Request.IntervalMs = a.delayMillis
				rt.buildClient()
			}
			a.rt = rt
			return nil
		},
	}

	flags := root.PersistentFlags()
	flags.StringVar(&a.logLevel, "log-level", "", "日志级别: trace/debug/info/warn/error (默认 info)")
	flags.StringVar(&a.logFormat, "log-format", "", "日志格式: text/json (默认 text)")
	flags.StringVar(&a.outputFmt, "format", "", fmt.Sprintf("输出格式: %s (默认 table)", joinFormats()))
	flags.BoolVar(&a.jsonOut, "json", false, "等价于 --format json")
	flags.BoolVar(&a.noRelogin, "no-relogin", false, "会话失效时不自动打开浏览器重新验证")
	flags.IntVar(&a.delayMillis, "delay", 0, "相邻请求的最小间隔毫秒数, 覆盖配置")

	root.AddCommand(
		a.newAuthCmd(),
		a.newSearchCmd(),
		a.newAdvCmd(),
		a.newExprCmd(),
		a.newSentenceCmd(),
		a.newAuthorCmd(),
		a.newRefineCmd(),
		a.newGroupCmd(),
		a.newDetailCmd(),
		a.newDownloadCmd(),
		a.newExportCmd(),
		a.newInfoCmd(),
		a.newConfigCmd(),
	)
	return root
}

// requireRuntime 取出已装配的运行时, 未装配时给出内部错误.
func (a *app) requireRuntime() (*Runtime, error) {
	if a.rt == nil {
		return nil, apperr.New(apperr.ExitFailure, "内部错误: 运行时尚未初始化")
	}
	return a.rt, nil
}

// loginRequired 给出统一的未登录提示.
func (a *app) loginRequired() error {
	if a.rt != nil {
		return a.rt.loginRequired()
	}
	return apperr.Session("尚未取得知网会话, 请先执行 cnki auth login")
}

// outputFormat 决定本次命令使用的输出格式.
//
// 命令行显式指定的 --format 或 --json 优先, 否则回落到配置文件取值.
func (a *app) outputFormat() (string, error) {
	if a.jsonOut {
		return output.FormatJSON, nil
	}
	if a.outputFmt != "" {
		if !output.ValidFormat(a.outputFmt) {
			return "", apperr.Usage("未知输出格式 %q, 可用值: %s", a.outputFmt, joinFormats())
		}
		return a.outputFmt, nil
	}
	if a.rt != nil && a.rt.Config.Output.Format != "" {
		return a.rt.Config.Output.Format, nil
	}
	return output.FormatTable, nil
}

// joinFormats 把可用输出格式拼成提示串.
func joinFormats() string {
	out := ""
	for i, f := range output.Formats {
		if i > 0 {
			out += ", "
		}
		out += f
	}
	return out
}
