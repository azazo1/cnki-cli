package taxonomy

import "strings"

// 匹配模式常量, 对应知网字段下拉里每项的默认检索方式.
//
// 取值来自结果页 .sort-list 中每个字段 li 内层 a 标签的 value 属性.
const (
	MatchTopRank = "TOPRANK" // 主题相关度优先
	MatchFuzzy   = "FUZZY"   // 模糊匹配
	MatchDefault = "DEFAULT" // 默认
	MatchSuffix  = "SUFFIX"  // 后缀匹配, 用于分类号
	MatchPrefix  = "PREFIX"  // 前缀匹配, 用于 DOI
	MatchExact   = "EXACT"   // 精确匹配
)

// Field 描述知网的一个检索字段.
type Field struct {
	// Name 是中文名, 与知网界面一致.
	Name string
	// Korder 是知网请求参数 korder 的取值.
	Korder string
	// Match 是该字段在界面上默认使用的匹配模式.
	Match string
}

// Fields 是知网一框式与高级检索支持的全部字段.
//
// 顺序与知网字段下拉一致. Korder 取值来自结果页 #selectfield 与
// .sort-list li[data-val] 的实测值.
var Fields = []Field{
	{Name: "主题", Korder: "SU", Match: MatchTopRank},
	{Name: "篇关摘", Korder: "TKA", Match: MatchFuzzy},
	{Name: "关键词", Korder: "KY", Match: MatchDefault},
	{Name: "篇名", Korder: "TI", Match: MatchFuzzy},
	{Name: "全文", Korder: "FT", Match: MatchFuzzy},
	{Name: "作者", Korder: "AU", Match: MatchDefault},
	{Name: "第一作者", Korder: "FI", Match: MatchDefault},
	{Name: "通讯作者", Korder: "RP", Match: MatchDefault},
	{Name: "作者单位", Korder: "AF", Match: MatchFuzzy},
	{Name: "基金", Korder: "FU", Match: MatchFuzzy},
	{Name: "摘要", Korder: "AB", Match: MatchFuzzy},
	{Name: "小标题", Korder: "CO", Match: MatchFuzzy},
	{Name: "参考文献", Korder: "RF", Match: MatchDefault},
	{Name: "分类号", Korder: "CLC", Match: MatchSuffix},
	{Name: "文献来源", Korder: "LY", Match: MatchFuzzy},
	{Name: "DOI", Korder: "DOI", Match: MatchPrefix},
}

// DefaultFieldKorder 是未显式指定字段时使用的检索字段.
const DefaultFieldKorder = "SU"

// Logic 是检索条件之间的布尔关系.
//
// 数值与知网 QueryJson 中 Logic 字段一致.
type Logic int

// 布尔关系取值.
const (
	LogicAnd  Logic = 0
	LogicOr   Logic = 1
	LogicNot  Logic = 2
	LogicNear Logic = 3 // 同一句
	LogicSen  Logic = 4 // 同一段
)

// Symbol 返回该布尔关系的符号形式, 供构造专业检索表达式使用.
func (l Logic) Symbol() string {
	switch l {
	case LogicOr:
		return "OR"
	case LogicNot:
		return "NOT"
	case LogicNear:
		return "NEAR"
	case LogicSen:
		return "SEN"
	default:
		return "AND"
	}
}

// ParseLogic 解析用户输入的布尔关系, 无法识别时返回 LogicAnd 与 false.
func ParseLogic(text string) (Logic, bool) {
	switch strings.ToUpper(strings.TrimSpace(text)) {
	case "AND", "与", "并且", "和":
		return LogicAnd, true
	case "OR", "或", "或者":
		return LogicOr, true
	case "NOT", "非", "不包含":
		return LogicNot, true
	case "NEAR", "同一句":
		return LogicNear, true
	case "SEN", "同一段":
		return LogicSen, true
	default:
		return LogicAnd, false
	}
}

// OperatorType 是单个检索项内部的匹配运算.
//
// 数值取自知网页面上 cnkiSearch.operatorType 的实测值, 原样保留其编号,
// 因为该编号会直接出现在 QueryJson 的 Operator 字段里.
type OperatorType int

// 匹配运算取值, 与知网 operatorType 一一对应.
const (
	OpDefault        OperatorType = 0  // 默认
	OpTopRank        OperatorType = 1  // 主题相关度优先
	OpFuzzy          OperatorType = 2  // 模糊
	OpGreaterThan    OperatorType = 3  // 大于
	OpGreaterOrEqual OperatorType = 4  // 大于等于
	OpLessThan       OperatorType = 5  // 小于
	OpLessOrEqual    OperatorType = 6  // 小于等于
	OpBetween        OperatorType = 7  // 区间
	OpFrequency      OperatorType = 8  // 词频
	OpPrefix         OperatorType = 9  // 前缀
	OpSuffix         OperatorType = 10 // 后缀
	OpContains       OperatorType = 11 // 包含
	OpNear           OperatorType = 12 // 同一句
	OpSentence       OperatorType = 13 // 同一段
	OpIs             OperatorType = 14 // 是
	OpFuzzyFrequency OperatorType = 15 // 模糊词频
)

// Name 返回该匹配运算对应的知网界面用词.
//
// 界面上提交的 Operator 是这类字符串而非数字, 例如一框式检索提交 "TOPRANK".
// 专业检索则提交数字 0.
func (o OperatorType) Name() string {
	switch o {
	case OpTopRank:
		return "TOPRANK"
	case OpFuzzy:
		return "FUZZY"
	case OpGreaterThan:
		return "GT"
	case OpGreaterOrEqual:
		return "GE"
	case OpLessThan:
		return "LT"
	case OpLessOrEqual:
		return "LE"
	case OpBetween:
		return "BETWEEN"
	case OpFrequency:
		return "FREQUENCY"
	case OpPrefix:
		return "PREFIX"
	case OpSuffix:
		return "SUFFIX"
	case OpContains:
		return "CONTAINS"
	case OpNear:
		return "NEAR"
	case OpSentence:
		return "SENTENCE"
	case OpIs:
		return "IS"
	case OpFuzzyFrequency:
		return "FUZZYFREQUENCY"
	default:
		return "DEFAULT"
	}
}

// ParseOperator 解析用户输入的匹配方式, 无法识别时返回 false.
func ParseOperator(text string) (OperatorType, bool) {
	switch strings.ToUpper(strings.TrimSpace(text)) {
	case "默认", "DEFAULT":
		return OpDefault, true
	case "主题", "TOPRANK", "相关度":
		return OpTopRank, true
	case "模糊", "FUZZY":
		return OpFuzzy, true
	case "大于", "GT":
		return OpGreaterThan, true
	case "大于等于", "GE":
		return OpGreaterOrEqual, true
	case "小于", "LT":
		return OpLessThan, true
	case "小于等于", "LE":
		return OpLessOrEqual, true
	case "区间", "BETWEEN":
		return OpBetween, true
	case "词频", "FREQUENCY":
		return OpFrequency, true
	case "前缀", "PREFIX":
		return OpPrefix, true
	case "后缀", "SUFFIX":
		return OpSuffix, true
	case "包含", "CONTAINS":
		return OpContains, true
	case "同一句", "NEAR":
		return OpNear, true
	case "同一段", "SENTENCE":
		return OpSentence, true
	case "是", "IS":
		return OpIs, true
	case "模糊词频", "FUZZYFREQUENCY":
		return OpFuzzyFrequency, true
	default:
		return OpDefault, false
	}
}

// OperatorOfMatch 把字段的默认匹配模式转换为匹配运算.
func OperatorOfMatch(match string) OperatorType {
	switch match {
	case MatchTopRank:
		return OpTopRank
	case MatchFuzzy:
		return OpFuzzy
	case MatchPrefix:
		return OpPrefix
	case MatchSuffix:
		return OpSuffix
	case MatchExact:
		return OpDefault
	default:
		return OpDefault
	}
}

// LookupField 按中文名或 korder 查找字段, 不区分大小写.
func LookupField(key string) (Field, bool) {
	key = strings.TrimSpace(key)
	if key == "" {
		return Field{}, false
	}
	for _, f := range Fields {
		if f.Name == key || strings.EqualFold(f.Korder, key) {
			return f, true
		}
	}
	return Field{}, false
}

// FieldByKorder 按 korder 查找字段.
func FieldByKorder(korder string) (Field, bool) { return LookupField(korder) }

// SortField 描述结果排序方式.
type SortField struct {
	// Name 是中文说明.
	Name string
	// Field 是对应知网参数 sortField 的取值.
	Field string
	// Kind 是对应知网参数 sortType 的取值, DESC 表示由高到低.
	Kind string
}

// SortFields 是知网支持的排序方式.
var SortFields = []SortField{
	{Name: "相关度", Field: "", Kind: ""},
	{Name: "发表时间", Field: "PT", Kind: "DESC"},
	{Name: "被引", Field: "CF", Kind: "DESC"},
	{Name: "下载", Field: "DF", Kind: "DESC"},
}

// LookupSort 按名称或参数值查找排序方式.
func LookupSort(key string) (SortField, bool) {
	key = strings.TrimSpace(key)
	if key == "" {
		return SortFields[0], true
	}
	upper := strings.ToUpper(key)
	for _, s := range SortFields {
		if s.Name == key || (s.Field != "" && strings.EqualFold(s.Field, key)) {
			return s, true
		}
	}
	switch upper {
	case "RELEVANCE", "SCORE":
		return SortFields[0], true
	case "DATE", "TIME", "PUBDATE":
		return SortFields[1], true
	case "CITED", "CITE":
		return SortFields[2], true
	case "DOWNLOAD", "DOWN":
		return SortFields[3], true
	}
	return SortField{}, false
}
