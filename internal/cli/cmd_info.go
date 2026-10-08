package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/azazo1/cnki-cli/internal/cnki"
	"github.com/azazo1/cnki-cli/internal/config"
	"github.com/azazo1/cnki-cli/internal/taxonomy"
)

// newInfoCmd 构造元信息查询命令.
func (a *app) newInfoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info",
		Short: "查询知网的库, 字段, 分组等元信息",
		Long:  "查询知网的库, 检索字段, 分组维度与导出格式等元信息, 便于构造其它命令的参数.",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "dbs",
			Short: "列出可检索的文献库",
			RunE: func(cmd *cobra.Command, _ []string) error {
				rows := make([][]string, 0, len(taxonomy.Databases))
				for _, db := range taxonomy.Databases {
					rows = append(rows, []string{db.Name, db.Classid, db.Resource, db.Group})
				}
				return writeSimpleTable(a, []string{"库名", "Classid", "Resource", "库族"}, rows)
			},
		},
		&cobra.Command{
			Use:   "fields",
			Short: "列出检索字段",
			RunE: func(cmd *cobra.Command, _ []string) error {
				rows := make([][]string, 0, len(taxonomy.Fields))
				for _, f := range taxonomy.Fields {
					rows = append(rows, []string{f.Name, f.Korder, f.Match})
				}
				return writeSimpleTable(a, []string{"字段", "korder", "默认匹配"}, rows)
			},
		},
		&cobra.Command{
			Use:   "groups",
			Short: "列出分组维度",
			RunE: func(cmd *cobra.Command, _ []string) error {
				rows := make([][]string, 0, len(cnki.GroupDimensions))
				for alias, id := range cnki.GroupDimensions {
					rows = append(rows, []string{alias, id})
				}
				sort.Slice(rows, func(i, j int) bool { return rows[i][0] < rows[j][0] })
				return writeSimpleTable(a, []string{"别名", "groupId"}, rows)
			},
		},
		&cobra.Command{
			Use:   "sorts",
			Short: "列出排序方式",
			RunE: func(cmd *cobra.Command, _ []string) error {
				rows := make([][]string, 0, len(taxonomy.SortFields))
				for _, s := range taxonomy.SortFields {
					rows = append(rows, []string{s.Name, s.Field, s.Kind})
				}
				return writeSimpleTable(a, []string{"排序", "sortField", "sortType"}, rows)
			},
		},
		&cobra.Command{
			Use:   "formats",
			Short: "列出题录导出格式",
			RunE: func(cmd *cobra.Command, _ []string) error {
				rows := make([][]string, 0, len(taxonomy.ExportFormats))
				for _, f := range taxonomy.ExportFormats {
					source := "知网"
					if f.Local {
						source = "本地生成"
					}
					rows = append(rows, []string{f.Name, f.Alias, f.DisplayMode, source})
				}
				return writeSimpleTable(a, []string{"格式", "别名", "displaymode", "来源"}, rows)
			},
		},
		&cobra.Command{
			Use:   "paths",
			Short: "显示配置文件与会话文件位置",
			RunE: func(cmd *cobra.Command, _ []string) error {
				fmt.Fprintf(a.opts.Stdout, "配置目录   %s\n", config.Dir())
				fmt.Fprintf(a.opts.Stdout, "配置文件   %s\n", config.ConfigPath())
				fmt.Fprintf(a.opts.Stdout, "会话文件   %s\n", config.SessionPath())
				fmt.Fprintf(a.opts.Stdout, "缓存目录   %s\n", config.CacheDir())
				fmt.Fprintf(a.opts.Stdout, "浏览器profile %s\n", config.ChromeProfileDir())
				return nil
			},
		},
	)
	return cmd
}

// newConfigCmd 构造配置管理命令.
func (a *app) newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "查看配置文件位置与当前取值",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "path",
			Short: "打印配置文件路径",
			RunE: func(cmd *cobra.Command, _ []string) error {
				fmt.Fprintln(a.opts.Stdout, config.ConfigPath())
				return nil
			},
		},
		&cobra.Command{
			Use:   "show",
			Short: "打印当前生效的配置",
			RunE: func(cmd *cobra.Command, _ []string) error {
				rt, err := a.requireRuntime()
				if err != nil {
					return err
				}
				cfg := rt.Config
				fmt.Fprintf(a.opts.Stdout, "配置文件          %s\n", config.ConfigPath())
				fmt.Fprintf(a.opts.Stdout, "配置版本          %d\n", cfg.SchemaVersion)
				fmt.Fprintf(a.opts.Stdout, "请求间隔          %d ms\n", cfg.Request.IntervalMs)
				fmt.Fprintf(a.opts.Stdout, "请求超时          %d s\n", cfg.Request.TimeoutSec)
				fmt.Fprintf(a.opts.Stdout, "重试次数          %d\n", cfg.Request.Retries)
				fmt.Fprintf(a.opts.Stdout, "知网地址          %s\n", cfg.Request.BaseURL)
				fmt.Fprintf(a.opts.Stdout, "默认输出格式      %s\n", cfg.Output.Format)
				fmt.Fprintf(a.opts.Stdout, "默认每页条数      %d\n", cfg.Output.PageSize)
				fmt.Fprintf(a.opts.Stdout, "默认结果上限      %d\n", cfg.Output.Limit)
				fmt.Fprintf(a.opts.Stdout, "Chrome 路径       %s\n", orDefault(cfg.Browser.ChromePath, "(自动探测)"))
				fmt.Fprintf(a.opts.Stdout, "无头模式          %t\n", cfg.Browser.Headless)
				fmt.Fprintf(a.opts.Stdout, "验证等待超时      %d s\n", cfg.Browser.VerifyTimeoutSec)
				fmt.Fprintf(a.opts.Stdout, "会话失效策略      %s\n", cfg.Session.OnExpired)
				return nil
			},
		},
	)
	return cmd
}

// writeSimpleTable 输出一个不带语义的等宽表格.
func writeSimpleTable(a *app, header []string, rows [][]string) error {
	widths := make([]int, len(header))
	for i, h := range header {
		widths[i] = displayLen(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && displayLen(cell) > widths[i] {
				widths[i] = displayLen(cell)
			}
		}
	}
	writeRow := func(cells []string) {
		parts := make([]string, 0, len(widths))
		for i, w := range widths {
			cell := ""
			if i < len(cells) {
				cell = cells[i]
			}
			parts = append(parts, cell+strings.Repeat(" ", maxInt(0, w-displayLen(cell))))
		}
		fmt.Fprintln(a.opts.Stdout, strings.TrimRight(strings.Join(parts, "  "), " "))
	}
	writeRow(header)
	seps := make([]string, len(widths))
	for i, w := range widths {
		seps[i] = strings.Repeat("-", w)
	}
	writeRow(seps)
	for _, row := range rows {
		writeRow(row)
	}
	return nil
}

// displayLen 估算字符串显示宽度, 中日韩字符按两格计.
func displayLen(text string) int {
	width := 0
	for _, r := range text {
		if r > 0x1100 && (r <= 0x115F ||
			(r >= 0x2E80 && r <= 0xA4CF) ||
			(r >= 0xAC00 && r <= 0xD7A3) ||
			(r >= 0xF900 && r <= 0xFAFF) ||
			(r >= 0xFE30 && r <= 0xFE6F) ||
			(r >= 0xFF00 && r <= 0xFF60) ||
			(r >= 0xFFE0 && r <= 0xFFE6)) {
			width += 2
			continue
		}
		width++
	}
	return width
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func orDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
