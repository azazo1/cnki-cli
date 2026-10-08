package taxonomy

// SearchType 是知网的检索方式编号.
//
// 取值来自知网前端 SearchTypeId 的实测值, 原样保留其编号,
// 因为它会直接出现在 QueryJson 的 SearchType 字段里.
type SearchType int

// 检索方式取值.
const (
	// SearchAdv 是高级检索, 多个字段条件以 AND/OR/NOT 组合.
	SearchAdv SearchType = 1
	// SearchSimple 是一框式检索, 单输入框加字段选择.
	SearchSimple SearchType = 2
	// SearchAuthor 是作者发文检索.
	SearchAuthor SearchType = 3
	// SearchExpert 是专业检索, 直接书写检索表达式.
	SearchExpert SearchType = 4
	// SearchSentence 是句子检索, 限定同一句或同一段.
	SearchSentence SearchType = 5
)

// Name 返回检索方式的中文名.
func (s SearchType) Name() string {
	switch s {
	case SearchAdv:
		return "高级检索"
	case SearchSimple:
		return "一框式检索"
	case SearchAuthor:
		return "作者发文检索"
	case SearchExpert:
		return "专业检索"
	case SearchSentence:
		return "句子检索"
	default:
		return "未知"
	}
}

// SearchFrom 是检索来源编号, 用于让知网在检索历史里正确归类.
//
// 取值来自知网前端 SearchFromId 的实测值.
type SearchFrom int

// 检索来源取值.
const (
	FromSearch        SearchFrom = 1
	FromGroupSearch   SearchFrom = 2
	FromResult        SearchFrom = 3 // 结果中检索
	FromPage          SearchFrom = 4
	FromSort          SearchFrom = 5
	FromChangeDB      SearchFrom = 6
	FromDisplayModel  SearchFrom = 7
	FromNaviSearch    SearchFrom = 8
	FromHistory       SearchFrom = 9
	FromComboHistory  SearchFrom = 10
	FromCrossDBChange SearchFrom = 11
	FromChangeLang    SearchFrom = 12
	FromNaviQuery     SearchFrom = 13
	FromGroup         SearchFrom = 99
)

// ExtendType 是检索式的扩展类型.
type ExtendType int

// 扩展类型取值.
const (
	// ExtendNone 表示普通检索式.
	ExtendNone ExtendType = 0
	// ExtendExpert 表示专业检索表达式.
	ExtendExpert ExtendType = 1
	// ExtendComposite 表示复合检索式.
	ExtendComposite ExtendType = 2
)

// SourceCategory 是来源类别筛选, 对应期刊的来源级别.
type SourceCategory struct {
	// Name 是中文名.
	Name string
	// Code 是知网使用的标识.
	Code string
}

// SourceCategories 是知网支持的来源类别.
var SourceCategories = []SourceCategory{
	{Name: "SCI", Code: "SCI"},
	{Name: "EI", Code: "EI"},
	{Name: "北大核心", Code: "PKU"},
	{Name: "CSSCI", Code: "CSSCI"},
	{Name: "CSCD", Code: "CSCD"},
	{Name: "AMI", Code: "AMI"},
	{Name: "WJCI", Code: "WJCI"},
}

// PageSizeOptions 是知网允许的每页条数.
var PageSizeOptions = []int{10, 20, 50}
