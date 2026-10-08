package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/azazo1/cnki-cli/internal/apperr"
	"github.com/azazo1/cnki-cli/internal/cnki"
	"github.com/azazo1/cnki-cli/internal/model"
	"github.com/azazo1/cnki-cli/internal/output"
)

// newDetailCmd 构造文献详情命令.
func (a *app) newDetailCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "detail <详情页地址|v参数>",
		Short: "查看文献详情",
		Long: `查看文献详情.

参数可以是完整的知网详情页地址, 也可以是地址里 v 参数的值. 详情页地址
由 search 命令输出的 detail_url 字段给出.

详情包含题名, 作者, 摘要, 关键词, 基金, 分类号等信息.`,
		Example: `  cnki detail 'https://kns.cnki.net/kcms2/article/abstract?v=xxx'
  cnki search 深度学习 --json | jq -r '.articles[0].detail_url' | xargs cnki detail`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := a.requireRuntime()
			if err != nil {
				return err
			}
			if !rt.HasSession() {
				return a.loginRequired()
			}
			format, err := a.outputFormat()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			article, err := withSessionRetry(ctx, rt, func(ctx context.Context) (*model.Article, error) {
				return rt.Client.Detail(ctx, args[0])
			})
			if err != nil {
				return err
			}
			return renderDetail(a.opts.Stdout, format, article)
		},
	}
	return cmd
}

// renderDetail 按输出格式渲染文献详情.
func renderDetail(w interface{ Write([]byte) (int, error) }, format string, article *model.Article) error {
	if format == output.FormatJSON {
		return output.RenderArticles(w, format, []model.Article{*article})
	}
	fmt.Fprintf(w, "题名: %s\n", article.Title)
	if len(article.Authors) > 0 {
		fmt.Fprintf(w, "作者: %s\n", article.AuthorNames())
	}
	if article.Source != "" {
		fmt.Fprintf(w, "来源: %s\n", article.Source)
	}
	if article.Date != "" {
		fmt.Fprintf(w, "日期: %s\n", article.Date)
	}
	if article.DOI != "" {
		fmt.Fprintf(w, "DOI:  %s\n", article.DOI)
	}
	if article.Fund != "" {
		fmt.Fprintf(w, "基金: %s\n", article.Fund)
	}
	if article.CLC != "" {
		fmt.Fprintf(w, "分类号: %s\n", article.CLC)
	}
	if len(article.Keywords) > 0 {
		fmt.Fprintf(w, "关键词: %s\n", strings.Join(article.Keywords, "; "))
	}
	if article.DetailURL != "" {
		fmt.Fprintf(w, "链接: %s\n", article.DetailURL)
	}
	if article.Abstract != "" {
		fmt.Fprintf(w, "\n摘要:\n%s\n", article.Abstract)
	}
	return nil
}

// newExportCmd 构造题录导出命令.
func (a *app) newExportCmd() *cobra.Command {
	flags := &searchFlags{}
	var format string
	var out string

	cmd := &cobra.Command{
		Use:   "export <关键词>",
		Short: "检索并导出题录",
		Long: `检索并导出题录.

支持两类格式:

  本地生成   ris, bibtex
  知网导出   gbt7714, mla, apa, cnki, reference, refworks, endnote,
             noteexpress, notefirst, custom

本地生成的格式直接由检索结果渲染, 不依赖知网的导出服务. 知网导出的格式
需要文献的加密标识, 因此总是跟在一次检索之后, 且部分格式要求登录个人账号.

用 --out 写入文件, 省略则输出到标准输出.`,
		Example: `  cnki export 深度学习 --format bibtex --limit 20 --out refs.bib
  cnki export 大语言模型 --format ris --limit 50
  cnki export 神经网络 --format gbt7714 --limit 10 --out refs.txt`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := a.requireRuntime()
			if err != nil {
				return err
			}
			if !rt.HasSession() {
				return a.loginRequired()
			}
			spec, err := resolveExportFormat(format)
			if err != nil {
				return err
			}

			limit := flags.limit
			if limit <= 0 {
				limit = 20
			}

			keyword := strings.Join(args, " ")
			q, err := flags.buildQuery(func(opts cnki.SearchOptions) (*cnki.Query, error) {
				// 导出需要足够多的条数, 这里把每页调到上限.
				opts.PageSize = 50
				return cnki.SimpleQuery(keyword, flags.field, opts)
			})
			if err != nil {
				return err
			}

			ctx := cmd.Context()
			result, err := withSessionRetry(ctx, rt, func(ctx context.Context) (*model.SearchResult, error) {
				return rt.Client.SearchPages(ctx, q, limit)
			})
			if err != nil {
				return err
			}
			if len(result.Articles) == 0 {
				return apperr.New(apperr.ExitOK, "没有可导出的文献")
			}

			writer, closeFn, err := openOutput(a, out)
			if err != nil {
				return err
			}
			defer closeFn()

			if spec.Local {
				switch strings.ToLower(spec.Alias) {
				case "ris":
					return output.RenderRIS(writer, result.Articles)
				default:
					return output.RenderBibTeX(writer, result.Articles)
				}
			}

			// 知网导出路径: 需要结果页给出的加密标识.
			ids := make([]string, 0, len(result.Articles))
			for _, article := range result.Articles {
				if article.Filename != "" {
					ids = append(ids, article.Filename)
				}
			}
			if len(ids) == 0 {
				return apperr.Remote("检索结果中未包含可导出的文献标识, 无法使用知网导出")
			}
			searchInfo := cnki.BuildSearchInfo("总库", result.Query)
			body, err := withSessionRetry(ctx, rt, func(ctx context.Context) (string, error) {
				return rt.Client.Export(ctx, cnki.ExportRequest{
					DisplayMode: spec.DisplayMode,
					Filenames:   ids,
					SearchInfo:  searchInfo,
				})
			})
			if err != nil {
				return err
			}
			rt.Logger.Info("知网导出完成", "格式", spec.Name, "条数", len(ids), "字符", len(body))
			if _, err := fmt.Fprintln(writer, body); err != nil {
				return err
			}
			return nil
		},
	}
	flags.register(cmd, true)
	cmd.Flags().StringVar(&format, "export-format", "bibtex", "导出格式, 见 cnki info formats")
	cmd.Flags().StringVarP(&out, "out", "o", "", "输出文件路径, 省略则输出到标准输出")
	cmd.Flags().Lookup("limit").Usage = "导出条数上限"
	return cmd
}

// openOutput 打开输出目标, 未指定文件时使用标准输出.
func openOutput(a *app, path string) (writer interface {
	Write([]byte) (int, error)
}, closeFn func(), err error) {
	if strings.TrimSpace(path) == "" {
		return a.opts.Stdout, func() {}, nil
	}
	file, err := createFile(path)
	if err != nil {
		return nil, func() {}, apperr.Wrap(apperr.ExitFailure, err, "创建输出文件 %s", path)
	}
	return file, func() { closeQuietly(file) }, nil
}

// createFile 由 files.go 提供.
