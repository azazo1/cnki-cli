package cnki

import (
	"fmt"
	"strings"

	"github.com/azazo1/cnki-cli/internal/apperr"
	"github.com/azazo1/cnki-cli/internal/taxonomy"
)

// Refine 在当前检索式基础上追加收窄条件, 对应知网的"结果中检索".
//
// 收窄一律按 AND 连接, 不采纳条件自身携带的逻辑关系: 结果中检索的语义就是
// 在已有结果集内继续筛. 需要 OR 或 NOT 时应当直接书写完整检索式.
//
// 追加方式取决于检索式的形态:
//
//   - 结构化检索 (一框式, 高级检索, 作者发文检索) 往主题检索组里追加检索项,
//     与高级检索构造多条件查询走的是同一套机制.
//   - 专业检索的检索词本身就是表达式, 追加条件会先翻译成表达式片段, 再以
//     AND 接到原表达式后面, 并把原表达式括起来保住它的优先级. 这样也避免把
//     数字型与字符串型的匹配运算混进同一个检索组.
func (q *Query) Refine(conditions []Condition, expression string) (*Query, error) {
	expression = strings.TrimSpace(expression)
	conditions = forceAndLogic(conditions)
	if len(conditions) == 0 && expression == "" {
		return nil, apperr.Usage("请在上一轮结果中至少给出一个收窄条件")
	}

	refined := q.clone()
	// 标记来源为"结果中检索", 让知网的检索历史把这次操作归类正确.
	refined.opts.SearchFrom = taxonomy.FromResult

	if refined.searchType == taxonomy.SearchExpert {
		return refined.refineExpert(conditions, expression)
	}
	if expression != "" {
		return nil, apperr.Usage(
			"结构化检索式只接受 --cond 形式的收窄条件. " +
				"需要直接书写表达式时, 请先用 cnki expr 建立结果集, 再用 --expr 收窄")
	}
	return refined.refineStructured(conditions)
}

// forceAndLogic 把条件的逻辑关系统一改为 AND.
func forceAndLogic(conditions []Condition) []Condition {
	out := make([]Condition, 0, len(conditions))
	for _, cond := range conditions {
		cond.Logic = taxonomy.LogicAnd
		out = append(out, cond)
	}
	return out
}

// clone 复制检索式, 避免收窄时改动调用方持有的对象.
func (q *Query) clone() *Query {
	groups := make([]QueryGroup, len(q.groups))
	for i, group := range q.groups {
		items := make([]QueryItem, len(group.Items))
		copy(items, group.Items)
		children := make([]QueryGroup, len(group.ChildItems))
		copy(children, group.ChildItems)
		group.Items = items
		group.ChildItems = children
		groups[i] = group
	}
	return &Query{
		searchType:  q.searchType,
		groups:      groups,
		opts:        q.opts,
		description: q.description,
		products:    q.products,
	}
}

// refineStructured 往主题检索组追加检索项.
func (q *Query) refineStructured(conditions []Condition) (*Query, error) {
	items, parts, err := buildConditionItems(conditions)
	if err != nil {
		return nil, err
	}

	index := q.subjectGroupIndex()
	if index < 0 {
		return nil, apperr.Remote("上一轮检索式里没有可追加条件的检索组")
	}

	existing := q.groups[index].Items
	// 新增项与原检索式的最后一项之间也必须是 AND.
	if len(existing) > 0 {
		existing[len(existing)-1].Logic = int(taxonomy.LogicAnd)
	}
	q.groups[index].Items = append(existing, items...)
	q.description = joinRefined(q.description, strings.Join(parts, " AND "))
	return q, nil
}

// refineExpert 把收窄条件翻译成表达式片段, 拼接到原表达式上.
func (q *Query) refineExpert(conditions []Condition, expression string) (*Query, error) {
	terms := make([]string, 0, len(conditions)+1)
	parts := make([]string, 0, len(conditions)+1)
	for _, cond := range conditions {
		term, err := expertTerm(cond)
		if err != nil {
			return nil, err
		}
		terms = append(terms, term)
		f, _ := taxonomy.LookupField(cond.Field)
		parts = append(parts, fmt.Sprintf("%s=%s", f.Name, strings.TrimSpace(cond.Value)))
	}
	if expression != "" {
		terms = append(terms, "("+expression+")")
		parts = append(parts, expression)
	}

	groupIndex, itemIndex := q.expertItemIndex()
	if groupIndex < 0 {
		return nil, apperr.Remote("上一轮专业检索式里没有可追加条件的表达式")
	}

	base := strings.TrimSpace(q.groups[groupIndex].Items[itemIndex].Value)
	if base == "" {
		return nil, apperr.Remote("上一轮专业检索表达式为空")
	}
	q.groups[groupIndex].Items[itemIndex].Value =
		"(" + base + ") AND " + strings.Join(terms, " AND ")
	q.description = joinRefined(q.description, strings.Join(parts, " AND "))
	return q, nil
}

// expertTerm 把一个检索条件翻译成专业检索的表达式片段.
func expertTerm(cond Condition) (string, error) {
	value := strings.TrimSpace(cond.Value)
	if value == "" {
		return "", apperr.Usage("检索词不能为空")
	}
	f, ok := taxonomy.LookupField(cond.Field)
	if !ok {
		return "", apperr.Usage("未知的检索字段 %q, 可用字段见 cnki info fields", cond.Field)
	}

	// 专业检索只有等号与百分号两种比较: 字段默认按模糊或相关度匹配时用
	// 百分号表达包含语义, 其余用等号.
	operator := "="
	if !cond.CustomOperator {
		switch taxonomy.OperatorOfMatch(f.Match) {
		case taxonomy.OpFuzzy, taxonomy.OpTopRank:
			operator = "%"
		}
	}
	return fmt.Sprintf("%s%s(%s)", f.Korder, operator, quoteExpertValue(value)), nil
}

// quoteExpertValue 按知网的约定给检索词加引号.
//
// 检索词自身含单引号时改用双引号, 两种引号知网都接受, 因此不必做转义.
func quoteExpertValue(value string) string {
	if strings.Contains(value, "'") {
		return `"` + value + `"`
	}
	return "'" + value + "'"
}

// subjectGroupIndex 返回主题检索组的下标, 找不到时返回 -1.
func (q *Query) subjectGroupIndex() int {
	for i, group := range q.groups {
		if group.Key == GroupSubject {
			return i
		}
	}
	if len(q.groups) > 0 {
		return 0
	}
	return -1
}

// expertItemIndex 返回专业检索表达式所在的位置.
func (q *Query) expertItemIndex() (int, int) {
	for groupIndex, group := range q.groups {
		for itemIndex, item := range group.Items {
			if item.Field == FieldExpert {
				return groupIndex, itemIndex
			}
		}
	}
	return -1, -1
}

// joinRefined 把收窄条件追加到检索式描述后面.
func joinRefined(base string, addition string) string {
	base = strings.TrimSpace(base)
	addition = strings.TrimSpace(addition)
	switch {
	case base == "":
		return addition
	case addition == "":
		return base
	default:
		return base + " AND " + addition
	}
}
