package cnki

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/azazo1/cnki-cli/internal/apperr"
	"github.com/azazo1/cnki-cli/internal/taxonomy"
)

// 下列结构体精确对应知网 brief 接口的 QueryJson 参数.
//
// 字段顺序与命名刻意保持与知网前端一致, 便于与浏览器抓到的请求逐字比对.
// 之所以把 Operator 与 ExScope 声明为 any: 知网在不同检索方式下对这两个
// 字段使用不同类型 —— 一框式检索的 Operator 是 "TOPRANK" 这样的字符串,
// 专业检索则是数字 0; ExScope 同样在数字与字符串之间摇摆.

// QueryItem 是检索式中的一个检索项.
type QueryItem struct {
	// Key 仅在专业检索等特定形态下出现, 例如 "Expert".
	Key string `json:"Key,omitempty"`
	// Field 是检索字段, 取值为 korder, 专业检索下为 "EXPERT".
	Field string `json:"Field"`
	// Value 是检索词或检索表达式.
	Value string `json:"Value"`
	// Value2 是区间类运算的第二个端点.
	Value2 string `json:"Value2,omitempty"`
	// Operator 是匹配运算, 数字或字符串形态见包注释.
	Operator any `json:"Operator"`
	// Logic 是与同一组内下一项的关系.
	Logic int `json:"Logic"`
	// Title 是字段中文名, 知网用于界面回显.
	Title string `json:"Title"`
	// Vector 是向量检索参数, 知网固定下发空串.
	Vector string `json:"Vector,omitempty"`
	// Options 是知网的附加选项对象.
	Options map[string]any `json:"options,omitempty"`
}

// QueryGroup 是一组检索项.
type QueryGroup struct {
	// Key 是组名, 主题检索为 "Subject", 控制条件为 "ControlGroup".
	Key string `json:"Key"`
	// Title 是组标题.
	Title string `json:"Title"`
	// Logic 是与下一组的关系.
	Logic int `json:"Logic"`
	// Items 是组内检索项.
	Items []QueryItem `json:"Items"`
	// ChildItems 是嵌套子组.
	ChildItems []QueryGroup `json:"ChildItems"`
}

// QueryNode 是 QueryJson 的 QNode 部分.
type QueryNode struct {
	QGroup []QueryGroup `json:"QGroup"`
}

// QueryJSON 是知网 brief 接口要求的 QueryJson 结构.
type QueryJSON struct {
	Platform   string         `json:"Platform"`
	Resource   string         `json:"Resource"`
	Classid    string         `json:"Classid"`
	Products   string         `json:"Products"`
	QNode      QueryNode      `json:"QNode"`
	ExScope    any            `json:"ExScope"`
	SimpTrad   string         `json:"SimpTrad"`
	SearchType int            `json:"SearchType"`
	Rlang      string         `json:"Rlang"`
	KuaKuCode  string         `json:"KuaKuCode"`
	Expands    map[string]any `json:"Expands"`
	View       string         `json:"View"`
	SearchFrom int            `json:"SearchFrom"`
}

// 控制组名与字段名的常量, 与知网前端一致.
const (
	// GroupSubject 是主题检索组的组名.
	GroupSubject = "Subject"
	// GroupControl 是控制条件组的组名.
	GroupControl = "ControlGroup"
	// FieldExpert 是专业检索使用的字段标识.
	FieldExpert = "EXPERT"
	// ViewChangeDB 是知网默认的结果视图标识.
	ViewChangeDB = "changeDBCh"
)

// Condition 是高级检索中的一个条件行.
type Condition struct {
	// Field 是检索字段 korder.
	Field string
	// Value 是检索词.
	Value string
	// Operator 是匹配运算, 零值表示采用该字段的默认匹配方式.
	Operator taxonomy.OperatorType
	// Logic 是本条件与下一个条件的关系.
	Logic taxonomy.Logic
	// CustomOperator 表示 Operator 由调用方显式指定, 不再套用字段默认值.
	CustomOperator bool
}

// SearchOptions 是发起检索时除检索式以外的控制参数.
type SearchOptions struct {
	// Classid 是目标库的 Classid, 留空使用总库.
	Classid string
	// Resource 是目标库的 Resource 标识.
	Resource string
	// KuaKuCode 是跨库检索的库代码列表, 留空使用总库默认值.
	KuaKuCode string
	// Rlang 是语种, 留空为 CHINESE.
	Rlang string
	// Page 是页码, 从 1 开始.
	Page int
	// PageSize 是每页条数.
	PageSize int
	// SortField 与 SortType 是排序参数.
	SortField string
	SortType  string
	// DisplayMode 是结果展示模式, 知网默认 listmode.
	DisplayMode string
	// SearchFrom 是检索来源编号.
	SearchFrom taxonomy.SearchFrom

	// 时间范围与来源类别等筛选条件.
	FromDate    string
	ToDate      string
	UpdateRange string
	SourceCats  []string
	ExScope     bool
	SimpTrad    bool
}

// withDefaults 补全检索控制参数的缺省值.
func (o SearchOptions) withDefaults() SearchOptions {
	if o.Classid == "" {
		o.Classid = taxonomy.TotalLibraryClassid
	}
	if o.Resource == "" {
		o.Resource = "CROSSDB"
	}
	// 跨库代码只对总库检索有意义. 指定了具体库 (如学位论文) 时必须留空,
	// 否则知网会拒绝请求并把结果页退化成一句错误提示.
	// 实测: 单库检索下带上总库的跨库列表会从 21 万条结果变成 0 条.
	if o.KuaKuCode == "" && o.Classid == taxonomy.TotalLibraryClassid {
		o.KuaKuCode = taxonomy.TotalLibraryKuaKuCode
	}
	if o.Rlang == "" {
		o.Rlang = "CHINESE"
	}
	if o.Page <= 0 {
		o.Page = 1
	}
	if o.PageSize <= 0 {
		o.PageSize = 20
	}
	if o.DisplayMode == "" {
		o.DisplayMode = "listmode"
	}
	if o.SearchFrom == 0 {
		o.SearchFrom = taxonomy.FromSearch
	}
	return o
}

// totalLibrarySearchFrom 是知网要求的下拉描述串.
//
// 实测该参数不可省略: 缺失或乱填会被知网当作无效请求, 返回
// "查询对象结构错误". 取值形如知网界面上的范围说明.
const totalLibrarySearchFrom = "资源范围：总库;  中英文扩展;  时间范围：更新时间：不限;  "

// Query 是一次检索的完整描述, 可渲染为知网接口所需的表单参数.
type Query struct {
	// searchType 决定 QueryJson 的组装形态.
	searchType taxonomy.SearchType
	// groups 是检索组.
	groups []QueryGroup
	// opts 是控制参数.
	opts SearchOptions
	// description 是人类可读的检索式描述, 用于日志与结果回显.
	description string
	// products 是知网的期刊产品过滤串, 留空表示不过滤.
	products string
}

// SimpleQuery 构造一框式检索.
//
// field 取值为 korder, 留空则按主题检索.
func SimpleQuery(keyword string, field string, opts SearchOptions) (*Query, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, apperr.Usage("检索词不能为空")
	}
	target := taxonomy.DefaultFieldKorder
	title := "主题"
	match := taxonomy.MatchTopRank
	if field != "" {
		f, ok := taxonomy.LookupField(field)
		if !ok {
			return nil, apperr.Usage("未知的检索字段 %q, 可用字段见 cnki info fields", field)
		}
		target, title, match = f.Korder, f.Name, f.Match
	}

	item := QueryItem{
		Field:    target,
		Value:    keyword,
		Operator: taxonomy.OperatorOfMatch(match).Name(),
		Logic:    int(taxonomy.LogicAnd),
		Title:    title,
		Vector:   "",
	}
	return &Query{
		searchType: taxonomy.SearchSimple,
		groups: []QueryGroup{{
			Key:        GroupSubject,
			Title:      "",
			Logic:      int(taxonomy.LogicAnd),
			Items:      []QueryItem{item},
			ChildItems: []QueryGroup{},
		}},
		opts:        opts,
		description: fmt.Sprintf("%s=%s", title, keyword),
	}, nil
}

// AdvancedQuery 构造高级检索.
//
// 多个条件之间按各自携带的 Logic 组合, 语义与知网高级检索的多行条件一致.
func AdvancedQuery(conditions []Condition, opts SearchOptions) (*Query, error) {
	if len(conditions) == 0 {
		return nil, apperr.Usage("高级检索至少需要一个检索条件")
	}
	items := make([]QueryItem, 0, len(conditions))
	parts := make([]string, 0, len(conditions))
	for i, cond := range conditions {
		value := strings.TrimSpace(cond.Value)
		if value == "" {
			return nil, apperr.Usage("第 %d 个检索条件的检索词为空", i+1)
		}
		f, ok := taxonomy.LookupField(cond.Field)
		if !ok {
			return nil, apperr.Usage("第 %d 个检索条件使用了未知字段 %q", i+1, cond.Field)
		}
		operator := cond.Operator
		if !cond.CustomOperator {
			operator = taxonomy.OperatorOfMatch(f.Match)
		}
		items = append(items, QueryItem{
			Field:    f.Korder,
			Value:    value,
			Operator: operator.Name(),
			Logic:    int(cond.Logic),
			Title:    f.Name,
			Vector:   "",
		})
		if i > 0 {
			parts = append(parts, cond.Logic.Symbol())
		}
		parts = append(parts, fmt.Sprintf("%s=%s", f.Name, value))
	}
	return &Query{
		searchType: taxonomy.SearchAdv,
		groups: []QueryGroup{{
			Key:        GroupSubject,
			Title:      "",
			Logic:      int(taxonomy.LogicAnd),
			Items:      items,
			ChildItems: []QueryGroup{},
		}},
		opts:        opts,
		description: strings.Join(parts, " "),
	}, nil
}

// ExpertQuery 构造专业检索, 直接接受知网的检索表达式.
//
// 表达式语法与知网界面一致, 例如:
//
//	SU=('深度学习'+'神经网络') AND AU=('张三')
func ExpertQuery(expression string, opts SearchOptions) (*Query, error) {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return nil, apperr.Usage("专业检索表达式不能为空")
	}
	item := QueryItem{
		Key:      "Expert",
		Title:    "",
		Logic:    int(taxonomy.LogicAnd),
		Field:    FieldExpert,
		Operator: int(taxonomy.OpDefault),
		Value:    expression,
		Value2:   "",
		Options:  map[string]any{},
	}
	return &Query{
		searchType: taxonomy.SearchExpert,
		groups: []QueryGroup{
			{
				Key:        GroupSubject,
				Title:      "",
				Logic:      int(taxonomy.LogicAnd),
				Items:      []QueryItem{item},
				ChildItems: []QueryGroup{},
			},
			{
				Key:        GroupControl,
				Title:      "",
				Logic:      int(taxonomy.LogicAnd),
				Items:      []QueryItem{},
				ChildItems: []QueryGroup{},
			},
		},
		opts:        opts,
		description: expression,
	}, nil
}

// AuthorQuery 构造作者发文检索.
func AuthorQuery(author string, org string, opts SearchOptions) (*Query, error) {
	author = strings.TrimSpace(author)
	org = strings.TrimSpace(org)
	if author == "" && org == "" {
		return nil, apperr.Usage("作者发文检索至少需要作者或作者单位")
	}
	var items []QueryItem
	var parts []string
	if author != "" {
		f, _ := taxonomy.LookupField("AU")
		items = append(items, QueryItem{
			Field: f.Korder, Value: author,
			Operator: taxonomy.OpDefault.Name(), Logic: int(taxonomy.LogicAnd),
			Title: f.Name, Vector: "",
		})
		parts = append(parts, "作者="+author)
	}
	if org != "" {
		f, _ := taxonomy.LookupField("AF")
		items = append(items, QueryItem{
			Field: f.Korder, Value: org,
			Operator: taxonomy.OpFuzzy.Name(), Logic: int(taxonomy.LogicAnd),
			Title: f.Name, Vector: "",
		})
		parts = append(parts, "作者单位="+org)
	}
	return &Query{
		searchType: taxonomy.SearchAuthor,
		groups: []QueryGroup{{
			Key: GroupSubject, Title: "", Logic: int(taxonomy.LogicAnd),
			Items: items, ChildItems: []QueryGroup{},
		}},
		opts:        opts,
		description: strings.Join(parts, " "),
	}, nil
}

// SentenceQuery 构造句子检索, 用于查找同时包含两个词的句子或段落.
//
// 实现细节来自实测: 句子检索的检索项必须落在全文 (FT) 字段上, 两个检索词
// 分别放在同一个检索项的 Value 与 Value2, 匹配运算取 NEAR (同一句) 或
// SENTENCE (同一段). 若改用主题字段或拆成两个检索项, 知网会返回
// "查询对象结构错误, 没有指定检索分类".
func SentenceQuery(text1 string, text2 string, sameParagraph bool, opts SearchOptions) (*Query, error) {
	text1 = strings.TrimSpace(text1)
	text2 = strings.TrimSpace(text2)
	if text1 == "" || text2 == "" {
		return nil, apperr.Usage("句子检索需要两个检索词")
	}
	operator := taxonomy.OpNear
	label := "同一句"
	if sameParagraph {
		operator = taxonomy.OpSentence
		label = "同一段"
	}
	// 句子检索总在全文库范围内进行, 因此显式补齐库与跨库代码.
	if opts.Classid == "" {
		opts.Classid = taxonomy.TotalLibraryClassid
	}
	if opts.KuaKuCode == "" {
		opts.KuaKuCode = taxonomy.TotalLibraryKuaKuCode
	}
	if opts.Resource == "" {
		opts.Resource = "CROSSDB"
	}
	item := QueryItem{
		Field:    SentenceField,
		Value:    text1,
		Value2:   text2,
		Operator: int(operator),
		Logic:    int(taxonomy.LogicAnd),
		Title:    label,
		Vector:   "",
	}
	return &Query{
		searchType: taxonomy.SearchSentence,
		groups: []QueryGroup{{
			Key: GroupSubject, Title: "", Logic: int(taxonomy.LogicAnd),
			Items: []QueryItem{item}, ChildItems: []QueryGroup{},
		}},
		opts:        opts,
		description: fmt.Sprintf("%s: %s / %s", label, text1, text2),
	}, nil
}

// SentenceField 是句子检索使用的检索字段.
//
// 知网句子检索的界面文案是"在全文同一句话中", 对应字段即全文.
const SentenceField = "FT"

// Description 返回检索式的可读描述.
func (q *Query) Description() string { return q.description }

// SearchType 返回检索方式.
func (q *Query) SearchType() taxonomy.SearchType { return q.searchType }

// Options 返回本次检索的控制参数.
func (q *Query) Options() SearchOptions { return q.opts }

// SetPage 修改页码, 用于翻页复用同一个检索式.
func (q *Query) SetPage(page int) { q.opts.Page = page }

// QueryJSON 渲染知网要求的 QueryJson 结构.
func (q *Query) QueryJSON() QueryJSON {
	opts := q.opts.withDefaults()
	exScope := any("0")
	if opts.ExScope {
		exScope = 1
	}
	// 一框式检索下知网下发的是数字型 ExScope, 其余模式沿用字符串形式.
	if q.searchType == taxonomy.SearchSimple {
		if opts.ExScope {
			exScope = 1
		} else {
			exScope = 0
		}
	}
	simptrad := "0"
	if opts.SimpTrad {
		simptrad = "1"
	}
	return QueryJSON{
		Platform:   "",
		Resource:   opts.Resource,
		Classid:    opts.Classid,
		Products:   q.products,
		QNode:      QueryNode{QGroup: q.groups},
		ExScope:    exScope,
		SimpTrad:   simptrad,
		SearchType: int(q.searchType),
		Rlang:      opts.Rlang,
		KuaKuCode:  opts.KuaKuCode,
		Expands:    map[string]any{},
		View:       ViewChangeDB,
		SearchFrom: int(opts.SearchFrom),
	}
}

// MarshalQueryJSON 把检索式序列化为接口要求的 JSON 字符串.
func (q *Query) MarshalQueryJSON() (string, error) {
	payload, err := json.Marshal(q.QueryJSON())
	if err != nil {
		return "", apperr.Wrap(apperr.ExitFailure, err, "序列化检索式")
	}
	return string(payload), nil
}

// Form 渲染 brief 接口所需的完整表单参数.
//
// 参数集合与顺序来自对知网真实请求的抓取, searchFrom 与 aside 两个
// 描述性参数同样不可或缺.
func (q *Query) Form() (url.Values, error) {
	queryJSON, err := q.MarshalQueryJSON()
	if err != nil {
		return nil, err
	}
	opts := q.opts.withDefaults()

	form := url.Values{}
	form.Set("boolSearch", "true")
	form.Set("QueryJson", queryJSON)
	form.Set("pageNum", strconv.Itoa(opts.Page))
	form.Set("pageSize", strconv.Itoa(opts.PageSize))
	form.Set("sortField", opts.SortField)
	form.Set("sortType", opts.SortType)
	form.Set("dstyle", opts.DisplayMode)
	form.Set("productStr", q.products)
	form.Set("aside", q.Aside())
	form.Set("searchFrom", q.SearchFromDescription())
	form.Set("subject", "")
	form.Set("language", "")
	form.Set("uniplatform", "")
	form.Set("CurPage", "1")
	// 句子检索走 /brief/senquery, 知网要求显式带上该标志以区分普通检索.
	if q.searchType == taxonomy.SearchSentence {
		form.Set("sentenceSearch", "true")
	}
	return form, nil
}

// Aside 渲染结果页左侧的检索式提示串.
func (q *Query) Aside() string {
	if q.searchType == taxonomy.SearchExpert {
		return q.description
	}
	var parts []string
	for _, g := range q.groups {
		for _, item := range g.Items {
			if item.Field == FieldExpert {
				parts = append(parts, item.Value)
				continue
			}
			operator := "="
			if name, ok := item.Operator.(string); ok {
				switch name {
				case taxonomy.MatchFuzzy, taxonomy.MatchTopRank:
					operator = "%"
				case taxonomy.MatchPrefix:
					operator = "="
				}
			}
			parts = append(parts, fmt.Sprintf("%s%s'%s'", item.Field, operator, item.Value))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "(" + strings.Join(parts, " AND ") + ")"
}

// SearchFromDescription 返回知网要求的范围描述串.
//
// 目前恒为总库描述, 未来支持按库定制时可在此扩展.
func (q *Query) SearchFromDescription() string { return totalLibrarySearchFrom }
