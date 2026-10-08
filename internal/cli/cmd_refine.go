package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/azazo1/cnki-cli/internal/apperr"
	"github.com/azazo1/cnki-cli/internal/cnki"
	"github.com/azazo1/cnki-cli/internal/taxonomy"
)

// newRefineCmd 构造结果中检索命令.
func (a *app) newRefineCmd() *cobra.Command {
	flags := &searchFlags{}
	var (
		conds      []string
		expression string
	)

	cmd := &cobra.Command{
		Use:     "refine",
		Aliases: []string{"within"},
		Short:   "在上一轮检索结果中收窄",
		Long: `在上一轮检索结果中收窄, 对应知网的"结果中检索".

它会读回上一次文献检索的检索式, 把新条件以 AND 追加到上面再检索一遍,
因此不必重写整个检索式就能逐步筛出想要的文献. 连续执行 refine 会不断
收窄同一个结果集.

收窄条件一律按 AND 连接. 需要 OR 或 NOT 时请直接用 cnki adv 或 cnki expr
书写完整检索式, 那两种方式的组合语义更明确.

上一轮是专业检索式时, --cond 会被翻译成表达式片段接到原表达式后面;
也可以直接用 --expr 追加一段自己写的表达式. 结构化检索式 (一框式, 高级
检索, 作者发文检索) 只接受 --cond.

可用字段: 主题/篇名/关键词/作者/作者单位/基金/摘要/全文/参考文献/分类号/
文献来源/DOI 等, 完整列表见 cnki info fields.`,
		Example: `  cnki search 深度学习 --limit 50
  cnki refine --cond '作者单位=清华大学'
  cnki refine --cond '基金=国家自然科学基金'

  # 专业检索式同样可以接续
  cnki expr "SU=('大语言模型')"
  cnki refine --expr "LY='计算机学报'"`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rt, err := a.requireRuntime()
			if err != nil {
				return err
			}
			if !rt.HasSession() {
				return a.loginRequired()
			}

			base, err := cnki.LoadLastQuery()
			if err != nil {
				return err
			}

			conditions, err := parseConditions(conds)
			if err != nil {
				return err
			}

			q, err := base.Refine(conditions, expression)
			if err != nil {
				return err
			}

			// 分页与排序沿用本次命令行的取值; 库与语种等仍由上一轮检索式决定,
			// 因此不接收 --db 之类的选项.
			opts, err := flags.options()
			if err != nil {
				return err
			}
			q.SetPage(opts.Page)
			q.SetPageSize(opts.PageSize)
			q.SetSort(opts.SortField, opts.SortType)

			rt.Logger.Info("在上一轮结果中收窄", "原检索式", base.Description(), "新检索式", q.Description())
			return a.runSearch(cmd, q, flags)
		},
	}

	// 只挂分页, 排序与分组这类展示选项: 检索范围由上一轮决定.
	cmd.Flags().StringArrayVar(&conds, "cond", nil, "收窄条件, 形如 字段=检索词, 可重复")
	cmd.Flags().StringVar(&expression, "expr", "", "追加一段专业检索表达式, 只用于上一轮是专业检索式的情况")
	cmd.Flags().StringVar(&flags.sort, "sort", "", "排序: relevance/date/cited/download")
	cmd.Flags().IntVar(&flags.page, "page", 1, "页码, 从 1 开始")
	cmd.Flags().IntVar(&flags.size, "size", 20, "每页条数: 10/20/50")
	cmd.Flags().IntVar(&flags.limit, "limit", 0, "取回的结果上限, 0 表示只取一页")
	cmd.Flags().StringSliceVar(&flags.groups, "group", nil, "同时取回分组聚合: subject/source/discipline/year/author/organization/fund 等")
	return cmd
}

// parseConditions 解析形如 字段=检索词 的条件列表.
//
// 与 cnki adv 的 --cond 保持同一种写法, 两个命令之间可以互相搬用条件.
func parseConditions(raw []string) ([]cnki.Condition, error) {
	conditions := make([]cnki.Condition, 0, len(raw))
	for i, text := range raw {
		field, value, ok := strings.Cut(text, "=")
		if !ok {
			return nil, apperr.Usage("第 %d 个条件 %q 格式不正确, 应为 字段=检索词", i+1, text)
		}
		field = strings.TrimSpace(field)
		value = strings.TrimSpace(value)
		if field == "" || value == "" {
			return nil, apperr.Usage("第 %d 个条件 %q 的字段与检索词都不能为空", i+1, text)
		}
		conditions = append(conditions, cnki.Condition{
			Field: field,
			Value: value,
			Logic: taxonomy.LogicAnd,
		})
	}
	return conditions, nil
}
