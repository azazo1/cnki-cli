package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/azazo1/cnki-cli/internal/apperr"
	"github.com/azazo1/cnki-cli/internal/cnki"
	"github.com/azazo1/cnki-cli/internal/model"
)

// newDownloadCmd 构造全文下载命令.
func (a *app) newDownloadCmd() *cobra.Command {
	var (
		format string
		outDir string
		force  bool
	)

	cmd := &cobra.Command{
		Use:   "download <详情页地址>...",
		Short: "下载全文 (PDF 或 CAJ)",
		Long: `下载文献全文.

入参是文献详情页地址, 与 cnki detail 接受的形式一致, 可以一次给多个. 地址
可以从 cnki search --json 的 detail_url 字段取到.

全文下载需要相应权限: 机构订阅网内通常可以直接下载, 网外需要在浏览器窗口里
登录个人账号后重新取得会话. 没有权限时知网会把请求跳到登录页, 本命令会明确
报出这一点, 而不是把登录页当成文件存下来.

知网详情页还提供"原版阅读"与"HTML阅读", 那两个是在线阅读入口而不是文件,
因此不在本命令范围内. 已存在的同名文件默认跳过, 用 --force 覆盖.`,
		Example: `  cnki download 'https://kns.cnki.net/kcms2/article/abstract?v=xxx'
  cnki download '<地址1>' '<地址2>' --format caj --out ~/papers
  cnki search 深度学习 --limit 5 --json | jq -r '.articles[].detail_url' | xargs cnki download`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := a.requireRuntime()
			if err != nil {
				return err
			}
			if !rt.HasSession() {
				return a.loginRequired()
			}
			target, err := cnki.ParseDownloadFormat(format)
			if err != nil {
				return err
			}

			dir := strings.TrimSpace(outDir)
			if dir == "" {
				dir = "."
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return apperr.Wrap(apperr.ExitFailure, err, "创建输出目录 %s", dir)
			}

			ctx := cmd.Context()
			failed := 0
			for _, item := range args {
				err := a.downloadOne(ctx, item, target, dir, force)
				if err == nil {
					continue
				}
				// 缺权限是共性原因, 继续试其余条目只会重复同一条报错.
				if cnki.IsLoginRequired(err) {
					return apperr.New(apperr.ExitPermission,
						"下载 %s 全文需要相应权限. 机构订阅网内可直接下载; "+
							"网外请用 cnki auth login 在浏览器窗口里登录个人账号, 再重试",
						target.Name())
				}
				failed++
				cmd.PrintErrln("下载失败:", err)
			}
			if failed > 0 {
				return apperr.New(apperr.ExitFailure, "%d 个条目下载失败", failed)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&format, "format", "pdf", "全文格式: pdf 或 caj")
	cmd.Flags().StringVarP(&outDir, "out", "o", ".", "输出目录")
	cmd.Flags().BoolVar(&force, "force", false, "覆盖已存在的同名文件")
	return cmd
}

// downloadOne 下载单篇文献.
func (a *app) downloadOne(ctx context.Context, target string, format cnki.DownloadFormat, dir string, force bool) error {
	rt, err := a.requireRuntime()
	if err != nil {
		return err
	}

	path := cnki.DetailPath(target)
	if path == "" {
		return apperr.Usage("无法识别的详情页地址 %q", target)
	}

	// 只取一次详情页: 下载入口与文献标题都从这一份 HTML 里解析.
	detailHTML, err := withSessionRetry(ctx, rt, func(ctx context.Context) (string, error) {
		resp, err := rt.Client.Get(ctx, path, nil)
		if err != nil {
			return "", err
		}
		return resp.Text(), nil
	})
	if err != nil {
		return err
	}

	article, err := cnki.ParseDetail(detailHTML)
	if err != nil {
		return err
	}
	orderURL, err := cnki.ParseDownloadOrderURL(detailHTML, format)
	if err != nil {
		return err
	}

	result, err := withSessionRetry(ctx, rt, func(ctx context.Context) (*cnki.DownloadResult, error) {
		return rt.Client.DownloadFullText(ctx, orderURL, rt.Client.AbsURL(path))
	})
	if err != nil {
		return err
	}

	name := downloadFileName(article, result)
	full := filepath.Join(dir, name)
	if !force {
		if _, statErr := os.Stat(full); statErr == nil {
			fmt.Fprintf(a.opts.Stdout, "已存在, 跳过: %s\n", full)
			return nil
		}
	}
	if err := os.WriteFile(full, result.Data, 0o644); err != nil {
		return apperr.Wrap(apperr.ExitFailure, err, "写入文件 %s", full)
	}

	rt.Logger.Info("已下载全文", "题名", article.Title, "文件", full, "字节", len(result.Data))
	fmt.Fprintf(a.opts.Stdout, "已下载: %s (%d 字节)\n", full, len(result.Data))
	return nil
}

// downloadFileName 决定落盘的文件名.
//
// 优先用文献标题: 文件名里带标题, 以后在目录里一眼就能认出来.
func downloadFileName(article *model.Article, result *cnki.DownloadResult) string {
	extension := "." + string(result.Format)
	if title := strings.TrimSpace(article.Title); title != "" {
		return sanitizeFileName(title) + extension
	}
	if name := strings.TrimSpace(result.FileName); name != "" {
		return sanitizeFileName(strings.TrimSuffix(name, filepath.Ext(name))) + extension
	}
	return "cnki-fulltext" + extension
}
