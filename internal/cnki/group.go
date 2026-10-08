package cnki

import (
	"context"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/azazo1/cnki-cli/internal/apperr"
	"github.com/azazo1/cnki-cli/internal/model"
)

// PathGroupResult 是分组聚合接口地址.
const PathGroupResult = "/kns8s/group/result"

// PathGroupSingle 是单个分组展开接口地址.
const PathGroupSingle = "/kns8s/group/singleresult"

// 分组维度标识, 取值为知网结果页 #divGroup 中各 dl 的 groupid 属性.
//
// 部分维度由多段标识以 "|||" 连接构成, 表示同时请求多个子维度.
const (
	// GroupIDSubject 主题, 主要主题与次要主题.
	GroupIDSubject = "ZYZT|||CYZT"
	// GroupIDSourceCategory 来源类别, 如北大核心, CSCD, WJCI.
	GroupIDSourceCategory = "LYBSM"
	// GroupIDDiscipline 学科分类.
	GroupIDDiscipline = "CCL"
	// GroupIDResearchLevel 研究层次.
	GroupIDResearchLevel = "YJCC"
	// GroupIDYear 发表年度.
	GroupIDYear = "YE"
	// GroupIDDocType 文献类型.
	GroupIDDocType = "WXLX2"
	// GroupIDSource 文献来源.
	GroupIDSource = "WXLY"
	// GroupIDAuthor 作者, 文献量与 H 指数.
	GroupIDAuthor = "AUC|||HWZZ"
	// GroupIDOrganization 机构.
	GroupIDOrganization = "AFC"
	// GroupIDFund 基金.
	GroupIDFund = "FUC"
	// GroupIDOpenAccess OA 出版.
	GroupIDOpenAccess = "OA"
)

// GroupDimensions 是全部可选分组维度, 键为命令行别名.
var GroupDimensions = map[string]string{
	"subject":      GroupIDSubject,
	"source":       GroupIDSourceCategory,
	"discipline":   GroupIDDiscipline,
	"level":        GroupIDResearchLevel,
	"year":         GroupIDYear,
	"doctype":      GroupIDDocType,
	"journal":      GroupIDSource,
	"author":       GroupIDAuthor,
	"organization": GroupIDOrganization,
	"fund":         GroupIDFund,
	"oa":           GroupIDOpenAccess,
}

// DefaultGroupIDs 是未显式指定时请求的分组集合, 与知网结果页默认展开的一致.
const DefaultGroupIDs = GroupIDSubject + "," + GroupIDSourceCategory + "," + GroupIDDiscipline

// Groups 取回检索结果的分组聚合数据.
//
// groupIDs 为空时使用 DefaultGroupIDs. 返回结果按知网返回顺序排列.
func (c *Client) Groups(ctx context.Context, q *Query, groupIDs string) ([]model.Group, error) {
	if q == nil {
		return nil, apperr.Usage("检索式为空")
	}
	if strings.TrimSpace(groupIDs) == "" {
		groupIDs = DefaultGroupIDs
	}
	queryJSON, err := q.MarshalQueryJSON()
	if err != nil {
		return nil, err
	}

	form := url.Values{}
	form.Set("queryJson", queryJSON)
	form.Set("groupIds", groupIDs)
	form.Set("manageId", "")
	form.Set("aside", q.Aside())
	form.Set("subject", "")

	resp, err := c.PostForm(ctx, PathGroupResult, form)
	if err != nil {
		return nil, err
	}
	return ParseGroups(resp.Text())
}

// ParseGroups 解析分组接口返回的 HTML 片段.
func ParseGroups(html string) ([]model.Group, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, apperr.Wrap(apperr.ExitFailure, err, "解析分组 HTML")
	}

	groups := []model.Group{}
	doc.Find("dl[groupid]").Each(func(_ int, dl *goquery.Selection) {
		groupID, _ := dl.Attr("groupid")
		group := model.Group{
			Field: groupID,
			Label: groupLabel(dl),
			Items: []model.GroupItem{},
		}
		dl.Find("dd").Each(func(_ int, dd *goquery.Selection) {
			subLabel, _ := dd.Attr("tit")
			dd.Find("li").Each(func(_ int, li *goquery.Selection) {
				input := li.Find("input").First()
				// 知网把展示文本放在 text 属性, value 可能是中图分类号这类
				// 内部代码; 因此优先取 text, 缺失时再退回 value 与链接文本.
				value, _ := input.Attr("text")
				if strings.TrimSpace(value) == "" {
					value, _ = input.Attr("value")
				}
				if strings.TrimSpace(value) == "" {
					value = normalizeText(li.Find("a").First().Text())
				}
				count := parseCount(li.Find("span").First().Text())
				if count < 0 {
					count = parseCount(li.Text())
				}
				if strings.TrimSpace(value) == "" {
					return
				}
				if subLabel != "" && group.Label != "" && subLabel != group.Label {
					value = subLabel + " " + value
				}
				group.Items = append(group.Items, model.GroupItem{Value: value, Count: count})
			})
		})
		if len(group.Items) > 0 {
			groups = append(groups, group)
		}
	})
	return groups, nil
}

// groupLabel 抽取分组的可读名称.
func groupLabel(dl *goquery.Selection) string {
	if b := dl.Find("dt b").First(); b.Length() > 0 {
		return normalizeText(b.Text())
	}
	if dt := dl.Find("dt").First(); dt.Length() > 0 {
		return normalizeText(dt.Text())
	}
	return ""
}

// ResolveGroupIDs 把命令行给出的分组别名解析为知网的 groupIds 串.
//
// 接受别名 (subject, year ...), 也接受直接传入的原始 groupId.
func ResolveGroupIDs(names []string) (string, error) {
	if len(names) == 0 {
		return DefaultGroupIDs, nil
	}
	ids := make([]string, 0, len(names))
	for _, name := range names {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" {
			continue
		}
		if id, ok := GroupDimensions[key]; ok {
			ids = append(ids, id)
			continue
		}
		// 允许直接传原始 ID, 便于知网新增维度时无需改代码.
		// 判定要求纯 ASCII: 中文维度名经 ToUpper 后与自身相同, 用大小写
		// 比较会把任意中文都误当成合法 ID.
		if isRawGroupID(name) {
			ids = append(ids, name)
			continue
		}
		return "", apperr.Usage("未知的分组维度 %q, 可用值见 cnki info groups", name)
	}
	if len(ids) == 0 {
		return DefaultGroupIDs, nil
	}
	return strings.Join(ids, ","), nil
}

// isRawGroupID 判断字符串是否是知网的原始 groupId.
//
// 原始 ID 由 ASCII 大写字母, 数字与分隔竖线构成, 例如 ZYZT|||CYZT.
// 这个判定必须排除非 ASCII 字符, 否则任何中文维度名都会被误认为合法 ID,
// 从而让拼写错误静默通过.
func isRawGroupID(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	hasLetter := false
	for _, r := range text {
		switch {
		case r >= 'A' && r <= 'Z':
			hasLetter = true
		case r >= '0' && r <= '9', r == '|', r == '_':
		default:
			return false
		}
	}
	return hasLetter
}
