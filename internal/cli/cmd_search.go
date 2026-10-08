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
	"github.com/azazo1/cnki-cli/internal/taxonomy"
)

// searchFlags 是各类检索命令共用的选项.
type searchFlags struct {
	field  string
	db     string
	from   string
	to     string
	sort   string
	page   int
	size   int
	limit  int
	groups []string
}

// register 把共用选项挂到命令上.
//
// withGroup 控制是否注册 --group: group 子命令自己要用这个标志名表达
// 分组维度, 与其它命令"附带取回分组"的语义不同, 因此由它自行注册.
func (f *searchFlags) register(cmd *cobra.Command, withGroup bool) {
	flags := cmd.Flags()
	flags.StringVarP(&f.field, "field", "f", "", "检索字段: 主题/篇名/关键词/作者/作者单位/基金/摘要/全文/参考文献/分类号/文献来源/DOI 等")
	flags.StringVar(&f.db, "db", "", "文献库: 总库/学术期刊/学位论文/会议/报纸/年鉴/专利/标准/图书/成果 等")
	flags.StringVar(&f.from, "from", "", "起始日期, 形如 2020-01-01")
	flags.StringVar(&f.to, "to", "", "截止日期, 形如 2024-12-31")
	flags.StringVar(&f.sort, "sort", "", "排序: relevance/date/cited/download")
	flags.IntVar(&f.page, "page", 1, "页码, 从 1 开始")
	flags.IntVar(&f.size, "size", 20, "每页条数: 10/20/50")
	flags.IntVar(&f.limit, "limit", 0, "取回的结果上限, 0 表示只取一页")
	if withGroup {
		flags.StringSliceVar(&f.groups, "group", nil, "同时取回分组聚合: subject/source/discipline/year/author/organization/fund 等")
	}
}

// options 把命令行选项转换为知网检索控制参数.
func (f *searchFlags) options() (cnki.SearchOptions, error) {
	opts := cnki.SearchOptions{
		Page:     f.page,
		PageSize: f.size,
		FromDate: f.from,
		ToDate:   f.to,
	}

	if f.db != "" {
		// 库名可能带连字符简写, 例如 "学术期刊" 与 "journal".
		db, ok := taxonomy.LookupDatabase(f.db)
		if !ok {
			return opts, apperr.Usage("未知的文献库 %q, 可用值见 cnki info dbs", f.db)
		}
		opts.Classid = db.Classid
		opts.Resource = db.Resource
		// 选定具体库时不再使用总库的跨库代码.
		if db.Classid != taxonomy.TotalLibraryClassid {
			opts.KuaKuCode = ""
		}
	}

	if f.sort != "" {
		sort, ok := taxonomy.LookupSort(f.sort)
		if !ok {
			return opts, apperr.Usage("未知的排序方式 %q, 可用值: relevance/date/cited/download", f.sort)
		}
		opts.SortField = sort.Field
		opts.SortType = sort.Kind
	}

	for _, size := range taxonomy.PageSizeOptions {
		if size == f.size {
			return opts, nil
		}
	}
	return opts, apperr.Usage("每页条数只能是 10, 20 或 50, 当前为 %d", f.size)
}

// buildQuery 依据当前选项与检索内容构造检索式.
func (f *searchFlags) buildQuery(build func(opts cnki.SearchOptions) (*cnki.Query, error)) (*cnki.Query, error) {
	opts, err := f.options()
	if err != nil {
		return nil, err
	}
	return build(opts)
}

// runSearch 执行检索并按既有选项渲染结果.
//
// 这是所有检索命令的公共出口: 统一处理会话, 翻页, 分组与输出格式.
func (a *app) runSearch(cmd *cobra.Command, q *cnki.Query, flags *searchFlags) error {
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

	result, err := withSessionRetry(ctx, rt, func(ctx context.Context) (*model.SearchResult, error) {
		if flags.limit > 0 {
			return rt.Client.SearchPages(ctx, q, flags.limit)
		}
		return rt.Client.Search(ctx, q)
	})
	if err != nil {
		return err
	}

	// 有结果时才请求分组, 空结果集上的分组没有意义.
	if len(flags.groups) > 0 && len(result.Articles) > 0 {
		groupIDs, err := cnki.ResolveGroupIDs(flags.groups)
		if err != nil {
			return err
		}
		groups, err := rt.Client.Groups(ctx, q, groupIDs)
		if err != nil {
			// 分组失败不影响主结果, 只记录诊断信息.
			rt.Logger.Warn("获取分组失败", "err", err)
		}
		result.Groups = groups
	}

	rt.Logger.Info("检索完成",
		"检索式", result.Query,
		"命中", result.Total,
		"取回", len(result.Articles))

	limit := flags.limit
	if limit <= 0 {
		limit = flags.size
	}
	if err := output.RenderSearchResult(a.opts.Stdout, format, result, limit); err != nil {
		return err
	}
	if len(result.Groups) > 0 && format == output.FormatJSON {
		return nil
	}
	if len(result.Groups) > 0 {
		fmt.Fprintln(a.opts.Stdout)
		return output.RenderGroups(a.opts.Stdout, format, result.Groups)
	}
	return nil
}

// newSearchCmd 构造一框式检索命令.
func (a *app) newSearchCmd() *cobra.Command {
	flags := &searchFlags{}
	cmd := &cobra.Command{
		Use:   "search <关键词>",
		Short: "一框式检索",
		Long: `一框式检索.

默认按主题检索, 可通过 --field 切换检索字段. 多个关键词可以组合使用
知网的检索运算符:

  *  与, 例如 神经网络*自然语言
  +  或, 例如 神经网络+深度学习
  -  非, 例如 神经网络-卷积
  "" 短语, 例如 "深度学习"
  () 分组, 例如 (神经网络+深度学习)*综述

含运算符的检索词建议用半角单引号包起来, 例如 '8-姜酚'.`,
		Example: `  cnki search 深度学习
  cnki search 深度学习 --field 篇名 --sort cited --limit 50
  cnki search 深度学习 --db 学位论文 --group discipline --json`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			keyword := strings.Join(args, " ")
			q, err := flags.buildQuery(func(opts cnki.SearchOptions) (*cnki.Query, error) {
				return cnki.SimpleQuery(keyword, flags.field, opts)
			})
			if err != nil {
				return err
			}
			return a.runSearch(cmd, q, flags)
		},
	}
	flags.register(cmd, true)
	return cmd
}

// newAdvCmd 构造高级检索命令.
func (a *app) newAdvCmd() *cobra.Command {
	flags := &searchFlags{}
	var (
		conds []string
		logic string
	)
	cmd := &cobra.Command{
		Use:   "adv",
		Short: "高级检索",
		Long: `高级检索.

每个 --cond 指定一个检索条件, 形如 字段=检索词, 字段可用中文名或 korder.
多个条件默认以 AND 组合, 可用 --logic 改为 OR 或 NOT.

字段可用值: 主题(SU) 篇关摘(TKA) 关键词(KY) 篇名(TI) 全文(FT) 作者(AU)
第一作者(FI) 通讯作者(RP) 作者单位(AF) 基金(FU) 摘要(AB) 小标题(CO)
参考文献(RF) 分类号(CLC) 文献来源(LY) DOI(DOI).`,
		Example: `  cnki adv --cond '主题=深度学习' --cond '作者单位=清华大学'
  cnki adv --cond '篇名=大语言模型' --logic OR --cond '篇名=生成式'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(conds) == 0 {
				return apperr.Usage("高级检索至少需要一个 --cond 条件")
			}
			relation, ok := taxonomy.ParseLogic(logic)
			if !ok && logic != "" {
				return apperr.Usage("未知的逻辑关系 %q, 可用值: AND/OR/NOT/NEAR/SEN", logic)
			}
			// 最后一个条件的关系无意义, 统一置为 AND 以免知网误判.
			conditions := make([]cnki.Condition, 0, len(conds))
			for i, raw := range conds {
				field, value, ok := strings.Cut(raw, "=")
				if !ok {
					return apperr.Usage("条件 %q 格式不正确, 应为 字段=检索词", raw)
				}
				itemLogic := relation
				if i == len(conds)-1 {
					itemLogic = taxonomy.LogicAnd
				}
				if i == 0 {
					// 第一个条件不参与连接, 关系由它身后的条件决定.
					itemLogic = relation
					if len(conds) == 1 {
						itemLogic = taxonomy.LogicAnd
					}
				}
				conditions = append(conditions, cnki.Condition{
					Field: strings.TrimSpace(field),
					Value: strings.TrimSpace(value),
					Logic: itemLogic,
				})
			}
			_ = args
			q, err := flags.buildQuery(func(opts cnki.SearchOptions) (*cnki.Query, error) {
				return cnki.AdvancedQuery(conditions, opts)
			})
			if err != nil {
				return err
			}
			return a.runSearch(cmd, q, flags)
		},
	}
	flags.register(cmd, true)
	cmd.Flags().StringArrayVar(&conds, "cond", nil, "检索条件, 形如 字段=检索词, 可重复")
	cmd.Flags().StringVar(&logic, "logic", "AND", "条件之间的逻辑关系: AND/OR/NOT/NEAR/SEN")
	return cmd
}

// newExprCmd 构造专业检索命令.
func (a *app) newExprCmd() *cobra.Command {
	flags := &searchFlags{}
	cmd := &cobra.Command{
		Use:   "expr <检索表达式>",
		Short: "专业检索",
		Long: `专业检索.

直接书写知网的检索表达式, 支持与知网界面一致的运算符:

  =  等于      %  包含
  *  与        +  或
  -  非        () 分组
  AND OR NOT

字段标识: SU 主题, TI 篇名, KY 关键词, AB 摘要, FT 全文, AU 作者,
FI 第一作者, RP 通讯作者, AF 作者单位, FU 基金, LY 文献来源,
CLC 分类号, DOI, RF 参考文献.`,
		Example: `  cnki expr "SU=('深度学习'+'神经网络') AND AU=('张三')"
  cnki expr "TI='大语言模型' AND LY='计算机学报'"`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			expression := strings.Join(args, " ")
			q, err := flags.buildQuery(func(opts cnki.SearchOptions) (*cnki.Query, error) {
				return cnki.ExpertQuery(expression, opts)
			})
			if err != nil {
				return err
			}
			return a.runSearch(cmd, q, flags)
		},
	}
	flags.register(cmd, true)
	return cmd
}

// newSentenceCmd 构造句子检索命令.
func (a *app) newSentenceCmd() *cobra.Command {
	flags := &searchFlags{}
	var sameParagraph bool
	cmd := &cobra.Command{
		Use:   "sentence <词一> <词二>",
		Short: "句子检索",
		Long: `句子检索.

查找同时包含两个检索词的句子或段落, 适合定位把两个概念联系起来的具体
表述. 默认限定在同一句内, 加 --paragraph 改为同一段内.`,
		Example: `  cnki sentence 深度学习 医学影像
  cnki sentence 大语言模型 教育 --paragraph`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			q, err := flags.buildQuery(func(opts cnki.SearchOptions) (*cnki.Query, error) {
				return cnki.SentenceQuery(args[0], args[1], sameParagraph, opts)
			})
			if err != nil {
				return err
			}
			return a.runSearch(cmd, q, flags)
		},
	}
	flags.register(cmd, true)
	cmd.Flags().BoolVar(&sameParagraph, "paragraph", false, "限定在同一段内, 默认限定在同一句内")
	return cmd
}

// newAuthorCmd 构造作者发文检索命令.
func (a *app) newAuthorCmd() *cobra.Command {
	flags := &searchFlags{}
	var org string
	cmd := &cobra.Command{
		Use:   "author <作者名>",
		Short: "作者发文检索",
		Long: `作者发文检索.

按作者姓名检索其发表的全部文献, 可附加作者单位限定. 与 --field 作者 的
区别在于本命令额外支持单位限定与作者发文统计的筛选条件.`,
		Example: `  cnki author 张三
  cnki author 张三 --org 清华大学`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			author := ""
			if len(args) > 0 {
				author = args[0]
			}
			if author == "" && org == "" {
				return apperr.Usage("需要提供作者名或 --org 作者单位")
			}
			q, err := flags.buildQuery(func(opts cnki.SearchOptions) (*cnki.Query, error) {
				return cnki.AuthorQuery(author, org, opts)
			})
			if err != nil {
				return err
			}
			return a.runSearch(cmd, q, flags)
		},
	}
	flags.register(cmd, true)
	cmd.Flags().StringVar(&org, "org", "", "作者单位")
	return cmd
}

// newGroupCmd 构造分组聚合命令.
func (a *app) newGroupCmd() *cobra.Command {
	flags := &searchFlags{}
	var groups []string
	cmd := &cobra.Command{
		Use:   "group <关键词>",
		Short: "查看检索结果的分组聚合",
		Long: `查看检索结果的分组聚合.

只输出各维度的取值与命中量, 不输出文献列表. 可用维度:

  subject       主题 (主要主题与次要主题)
  source        来源类别 (北大核心 / CSSCI / CSCD 等)
  discipline    学科分类
  level         研究层次
  year          发表年度
  doctype       文献类型
  journal       文献来源
  author        作者
  organization  机构
  fund          基金
  oa            OA 出版`,
		Example: `  cnki group 深度学习 --group year --group discipline
  cnki group 大语言模型 --group source --json`,
		Args: cobra.MinimumNArgs(1),
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
			keyword := strings.Join(args, " ")
			q, err := flags.buildQuery(func(opts cnki.SearchOptions) (*cnki.Query, error) {
				return cnki.SimpleQuery(keyword, flags.field, opts)
			})
			if err != nil {
				return err
			}
			groupIDs, err := cnki.ResolveGroupIDs(groups)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			result, err := withSessionRetry(ctx, rt, func(ctx context.Context) ([]model.Group, error) {
				return rt.Client.Groups(ctx, q, groupIDs)
			})
			if err != nil {
				return err
			}
			return output.RenderGroups(a.opts.Stdout, format, result)
		},
	}
	flags.register(cmd, false)
	cmd.Flags().StringSliceVar(&groups, "group", []string{"subject", "year", "discipline"}, "要聚合的维度, 可重复")
	return cmd
}
