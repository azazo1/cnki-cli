package cnki

import (
	"context"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/azazo1/cnki-cli/internal/apperr"
	"github.com/azazo1/cnki-cli/internal/model"
)

// PathArticleAbstract 是文献详情页地址前缀.
//
// 完整地址形如 /kcms2/article/abstract?v=<加密标识>, 该加密标识由检索
// 结果页给出, 无法自行构造, 因此 detail 命令既接受完整地址也接受其中
// 的 v 参数.
const PathArticleAbstract = "/kcms2/article/abstract"

// Detail 取回并解析一篇文献的详情.
//
// target 可以是完整详情页地址, 也可以是 /kcms2/article/abstract 路径,
// 还可以是单独的 v 参数值.
func (c *Client) Detail(ctx context.Context, target string) (*model.Article, error) {
	path := DetailPath(target)
	if path == "" {
		return nil, apperr.Usage("无法识别的详情页地址 %q", target)
	}
	resp, err := c.Get(ctx, path, nil)
	if err != nil {
		return nil, err
	}
	article, err := ParseDetail(resp.Text())
	if err != nil {
		return nil, err
	}
	article.DetailURL = c.AbsURL(path)
	return article, nil
}

// DetailPath 把各种形式的详情页标识规整为请求路径.
//
// 接受完整详情页地址, 以 / 开头的路径, 以及裸的 v 参数. 无法识别时返回空串.
func DetailPath(target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return ""
	}
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		// 去掉协议与主机部分, 只保留路径和查询串.
		idx := strings.Index(target, "//")
		if idx >= 0 {
			rest := target[idx+2:]
			if slash := strings.Index(rest, "/"); slash >= 0 {
				return rest[slash:]
			}
			return ""
		}
		return target
	}
	if strings.HasPrefix(target, "/") {
		return target
	}
	// 视作裸的 v 参数.
	return PathArticleAbstract + "?v=" + target
}

// ParseDetail 从详情页 HTML 中抽取文献元数据.
func ParseDetail(html string) (*model.Article, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, apperr.Wrap(apperr.ExitFailure, err, "解析详情页 HTML")
	}

	article := &model.Article{}

	// 标题块可能带上 "附视频" 之类的后缀, 去掉以保证题录干净.
	rawTitle := normalizeText(doc.Find("h1").First().Text())
	article.Title = trimTitleSuffix(rawTitle)
	if article.Title == "" {
		// 退回到页面 title, 去掉站点后缀.
		pageTitle := normalizeText(doc.Find("title").First().Text())
		article.Title = strings.TrimSuffix(pageTitle, " - 中国知网")
	}
	if article.Title == "" {
		return nil, apperr.Remote("详情页中未找到文献标题, 知网可能已改版或该文献无访问权限")
	}

	// 作者块: 知网把姓名与单位都放在 a 标签里, 单位形如 "1.某某大学某学院",
	// 因此这里按"是否像人名"过滤, 单位另存.
	doc.Find("h3.author a, .author a").Each(func(_ int, a *goquery.Selection) {
		name := normalizeText(a.Text())
		if name == "" {
			return
		}
		if isLikelyOrganization(name) {
			if !containsAuthor(article.Organizations, name) {
				article.Organizations = append(article.Organizations, trimOrgIndex(name))
			}
			return
		}
		name = trimAuthorMarker(name)
		if name == "" {
			return
		}
		if !containsAuthor(article.Authors, name) {
			article.Authors = append(article.Authors, name)
		}
	})
	if len(article.Authors) == 0 {
		if raw := normalizeText(doc.Find("h3.author").First().Text()); raw != "" {
			for _, name := range strings.Fields(raw) {
				if isLikelyPersonName(name) {
					article.Authors = append(article.Authors, name)
				}
			}
		}
	}

	article.Abstract = normalizeText(doc.Find("#ChDivSummary").First().Text())
	if article.Abstract == "" {
		article.Abstract = normalizeText(doc.Find(".abstract-text").First().Text())
	}

	doc.Find(".keywords a, p.keywords a").Each(func(_ int, a *goquery.Selection) {
		if kw := normalizeText(a.Text()); kw != "" {
			article.Keywords = append(article.Keywords, strings.Trim(kw, ";；"))
		}
	})

	// 页面顶部的信息条通常包含 DOI 与来源.
	doc.Find(".top-tip a, .brief .top-tip a").Each(func(_ int, a *goquery.Selection) {
		text := normalizeText(a.Text())
		if strings.HasPrefix(strings.ToUpper(text), "10.") && article.DOI == "" {
			article.DOI = text
		}
	})

	article.Fund = extractFund(doc)
	article.CLC = extractLabeledValue(doc, "分类号")
	article.ISSN = extractLabeledValue(doc, "ISSN")

	// 顶部出处信息里还有单独的机构名, 在作者块没给出时补上.
	if len(article.Organizations) == 0 {
		if org := normalizeText(doc.Find(".orgn a").First().Text()); org != "" {
			article.Organizations = []string{org}
		}
	}

	return article, nil
}

// extractFund 抽取基金信息.
//
// 知网详情页的基金字段排版不固定, 有时是独立的 funds 区块, 有时混在
// 摘要上方的说明行里, 因此按区块优先, 文本兜底.
func extractFund(doc *goquery.Document) string {
	if node := doc.Find(".funds, p.funds, .fund").First(); node.Length() > 0 {
		if text := normalizeText(node.Text()); len(text) > 2 {
			return strings.TrimSpace(strings.TrimPrefix(text, "基金"))
		}
	}
	return extractLabeledValue(doc, "基金")
}

// isLikelyOrganization 判断作者块里的条目是否为机构而非人名.
//
// 知网的单位条目多以序号开头, 例如 "1.湖南工商大学新能源与环境学院".
func isLikelyOrganization(text string) bool {
	if text == "" {
		return false
	}
	if text[0] >= '0' && text[0] <= '9' {
		return true
	}
	for _, marker := range []string{"大学", "学院", "研究院", "研究所", "实验室", "中心", "医院", "公司", "集团", "school", "university", "institute"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

// trimOrgIndex 去掉机构前面的序号前缀.
func trimOrgIndex(text string) string {
	if idx := strings.Index(text, "."); idx > 0 && idx < 4 {
		allDigits := true
		for _, r := range text[:idx] {
			if r < '0' || r > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			return strings.TrimSpace(text[idx+1:])
		}
	}
	return text
}

// trimAuthorMarker 去掉作者名后面的单位角标.
//
// 知网详情页会把作者与单位编号写在一起, 形如 "苏长青1,2".
func trimAuthorMarker(text string) string {
	text = strings.TrimSpace(text)
	end := len(text)
	for end > 0 {
		r := rune(text[end-1])
		if (r >= '0' && r <= '9') || r == ',' || r == '，' {
			end--
			continue
		}
		break
	}
	trimmed := strings.TrimSpace(text[:end])
	if trimmed == "" {
		return text
	}
	return trimmed
}

// extractLabeledValue 在详情页中查找形如 "标签：值" 的字段.
//
// 知网详情页的元信息排版在不同文献类型间并不统一, 因此这里按标签文本
// 在整页范围内搜索, 而不是绑定具体选择器.
func extractLabeledValue(doc *goquery.Document, label string) string {
	var found string
	doc.Find("p, li, span, td").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		text := normalizeText(s.Text())
		if !strings.HasPrefix(text, label) {
			return true
		}
		value := strings.TrimSpace(strings.TrimPrefix(text, label))
		value = strings.TrimLeft(value, ":：")
		value = strings.TrimSpace(value)
		if value == "" {
			return true
		}
		// 只取冒号后的第一个字段, 避免把后续并列标签一起吞进来.
		if idx := strings.IndexAny(value, "："); idx > 0 && len(value) > 60 {
			value = value[:idx]
		}
		found = value
		return false
	})
	return found
}

// trimTitleSuffix 去掉知网标题块附加的形态后缀.
func trimTitleSuffix(title string) string {
	for _, suffix := range []string{" 附视频", " 附音频", " 免费", " 网络首发"} {
		title = strings.TrimSuffix(title, suffix)
	}
	return strings.TrimSpace(title)
}

// containsAuthor 判断作者是否已存在.
func containsAuthor(authors []string, name string) bool {
	for _, a := range authors {
		if a == name {
			return true
		}
	}
	return false
}

// isLikelyPersonName 粗略判断字符串是否像人名.
//
// 知网作者块会把单位编号以数字形式混在姓名里, 这里过滤纯数字与过长片段.
func isLikelyPersonName(text string) bool {
	if text == "" || len([]rune(text)) > 8 {
		return false
	}
	for _, r := range text {
		if r >= '0' && r <= '9' {
			return false
		}
	}
	return true
}
