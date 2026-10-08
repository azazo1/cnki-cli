package cnki

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/azazo1/cnki-cli/internal/apperr"
	"github.com/azazo1/cnki-cli/internal/model"
)

// 结果页 HTML 中的关键选择器.
//
// 取值来自对知网真实响应体的解析结果, 知网改版时集中在这里修正.
const (
	// selResultTable 是结果表格.
	selResultTable = "table.result-table-list"
	// selTotalCount 是总命中数所在元素.
	selTotalCount = "#countPageDiv em"
	// selPageMark 是形如 "1/300" 的页码标记.
	selPageMark = ".countPageMark"
	// selNoContent 是空结果提示.
	selNoContent = "p.no-content"
)

// nonDigit 用于从 "34,967" 这类文本中抽出数字.
var nonDigit = regexp.MustCompile(`[^0-9]`)

// ParseSearchResult 解析 brief 接口返回的结果页 HTML.
//
// 结果页是服务端渲染的静态 HTML, 不需要执行 JS 即可完整解析.
func ParseSearchResult(html string, page int, pageSize int) (*model.SearchResult, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, apperr.Wrap(apperr.ExitFailure, err, "解析结果页 HTML")
	}

	result := &model.SearchResult{
		Page:     page,
		PageSize: pageSize,
		Articles: []model.Article{},
	}

	if total := parseCount(doc.Find(selTotalCount).First().Text()); total >= 0 {
		result.Total = total
	}
	if mark := strings.TrimSpace(doc.Find(selPageMark).First().Text()); mark != "" {
		if _, after, ok := strings.Cut(mark, "/"); ok {
			result.PageCount = parseCount(after)
		}
	}

	table := doc.Find(selResultTable).First()
	if table.Length() == 0 {
		// 知网的提示元素同时用于两种情况: 正常的"无结果"与它对请求的拒绝.
		// 拒绝时原因写在 value 属性里, 例如 value="查询对象结构错误！"。
		// 若不加区分地返回空结果, 用户会以为是自己检索词没命中, 而实际上
		// 是检索式有问题, 这种静默失败会让排查变得很困难.
		if node := doc.Find(selNoContent).First(); node.Length() > 0 {
			if reason := noContentReason(node); reason != "" {
				return nil, apperr.Remote("知网拒绝了该检索请求: %s", reason)
			}
			return result, nil
		}
		return nil, apperr.Remote("结果页结构无法识别 (%s)", describePage(doc, html))
	}

	offset := (page - 1) * pageSize
	table.Find("tbody tr").Each(func(i int, row *goquery.Selection) {
		article := parseResultRow(row)
		if article.Title == "" {
			return
		}
		article.Index = offset + i + 1
		result.Articles = append(result.Articles, article)
	})

	return result, nil
}

// parseResultRow 解析结果表中的一行.
func parseResultRow(row *goquery.Selection) model.Article {
	var article model.Article

	// 篇名单元格同时承载标题, 详情链接与网络首发等标记.
	nameCell := row.Find("td.name").First()
	titleLink := nameCell.Find("a").First()
	article.Title = normalizeText(titleLink.Text())
	article.DetailURL, _ = titleLink.Attr("href")
	if article.DetailURL != "" {
		article.DetailURL = absURL(article.DetailURL)
	}
	if nameCell.Find("b.marktip").Length() > 0 {
		article.Type = normalizeText(nameCell.Find("b.marktip").First().Text())
	}
	if nameCell.Find("i.freeIdent").Length() > 0 {
		// 知网始终渲染这个标记, 仅在确有免费全文时才显示.
		// 因此必须看 style 里的 display 状态, 否则会把付费文献误判为有全文.
		article.HasFullTxt = isDisplayed(nameCell.Find("i.freeIdent").First())
	}

	// 作者单元格中每个作者是一个 a 标签, 以分号分隔.
	row.Find("td.author a").Each(func(_ int, a *goquery.Selection) {
		if name := normalizeText(a.Text()); name != "" {
			article.Authors = append(article.Authors, name)
		}
	})

	// 来源单元格是期刊或授予单位, 可能带 navi 链接.
	sourceCell := row.Find("td.source").First()
	article.Source = normalizeText(sourceCell.Text())

	article.Date = normalizeText(row.Find("td.date").First().Text())
	article.Dbname = normalizeText(row.Find("td.data").First().Text())
	article.CitedCount = parseCount(row.Find("td.cited").First().Text())
	if article.CitedCount < 0 {
		article.CitedCount = 0
	}
	article.DownCount = parseCount(row.Find("td.download").First().Text())
	if article.DownCount < 0 {
		article.DownCount = 0
	}

	// 序号格里的复选框 value 是知网内部的加密文件名, 用于后续导出与下载.
	if raw, ok := row.Find("td.seq input.cbItem").First().Attr("value"); ok {
		article.Filename = raw
	}
	// 详情链接的 v 参数即加密文件名, 作为兜底.
	if article.Filename == "" && article.DetailURL != "" {
		if idx := strings.Index(article.DetailURL, "v="); idx >= 0 {
			article.Filename = article.DetailURL[idx+2:]
		}
	}

	return article
}

// parseCount 从可能带千分位的文本中抽取整数.
//
// 返回 -1 表示未能解析出数字.
func parseCount(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return -1
	}
	digits := nonDigit.ReplaceAllString(text, "")
	if digits == "" {
		return -1
	}
	value, err := strconv.Atoi(digits)
	if err != nil {
		return -1
	}
	return value
}

// isDisplayed 判断元素是否真的可见.
//
// goquery 不做 CSS 计算, 这里只检查最常见的行内隐藏写法.
func isDisplayed(sel *goquery.Selection) bool {
	if sel.Length() == 0 {
		return false
	}
	style, _ := sel.Attr("style")
	normalized := strings.ToLower(strings.ReplaceAll(style, " ", ""))
	if strings.Contains(normalized, "display:none") {
		return false
	}
	if _, hidden := sel.Attr("hidden"); hidden {
		return false
	}
	return true
}

// noContentReason 取出知网在提示元素里给出的拒绝原因.
//
// 知网用同一个 p.no-content 同时表达两种情况: 正常的"无结果"与它拒绝
// 这次请求. 拒绝时原因写在 value 属性里, 例如 value="查询对象结构错误！";
// 正常无结果时 value 为空. 返回空串表示这是正常的无结果.
func noContentReason(node *goquery.Selection) string {
	return normalizeText(node.AttrOr("value", ""))
}

// shortPageBytes 是判定"短页面"的阈值.
//
// 正常结果页有上百 KB, 而错误页与提示页通常只有几 KB, 短页面直接把可见
// 文本带进错误信息, 有助于一眼看出知网到底返回了什么.
const shortPageBytes = 4096

// describePage 提取响应体的可诊断特征, 供解析失败时定位原因.
//
// 只说"未找到结果表格"无法区分是知网改版, 被限流, 还是需要登录, 因此把
// 最能说明问题的几处特征抽出来: 长度, 标题, 知网的提示文本与短页面正文.
func describePage(doc *goquery.Document, html string) string {
	parts := []string{fmt.Sprintf("长度 %d 字节", len(html))}

	if title := normalizeText(doc.Find("title").First().Text()); title != "" {
		parts = append(parts, fmt.Sprintf("标题 %q", title))
	}

	if hint := firstHintValue(doc); hint != "" {
		parts = append(parts, fmt.Sprintf("提示 %q", hint))
	}

	if len(html) <= shortPageBytes {
		if text := normalizeText(doc.Text()); text != "" {
			parts = append(parts, fmt.Sprintf("正文 %q", clipRunes(text, 160)))
		}
	}

	return strings.Join(parts, ", ")
}

// firstHintValue 找出页面上第一个带内容的非表单 value 属性.
//
// 知网习惯把错误原因放在 value 里, 但表单控件 (input 等) 的 value 是
// 正常数据, 需要排除, 否则会取到 classid 这类无关取值.
func firstHintValue(doc *goquery.Document) string {
	var hint string
	doc.Find("[value]").EachWithBreak(func(_ int, sel *goquery.Selection) bool {
		switch goquery.NodeName(sel) {
		case "input", "button", "select", "textarea", "option":
			return true
		}
		value := normalizeText(sel.AttrOr("value", ""))
		if value == "" {
			return true
		}
		hint = value
		return false
	})
	return hint
}

// clipRunes 按字符数截断文本.
//
// 按字节切会切出无效 UTF-8, 中文会变成乱码, 因此按 rune 处理.
func clipRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "..."
}

// normalizeText 压缩空白并去掉首尾空格.
func normalizeText(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// ParseSentenceResult 解析句子检索接口返回的结果页 HTML.
//
// 句子检索的结果页结构与普通检索完全不同: 没有结果表格, 而是每条结果一个
// .middle 区块, 其中 h5 是命中的原句, h6 里的链接指向出处文献, baseinfo
// 承载作者, 来源与日期.
func ParseSentenceResult(html string, page int, pageSize int) (*model.SearchResult, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, apperr.Wrap(apperr.ExitFailure, err, "解析句子检索结果 HTML")
	}

	result := &model.SearchResult{
		Page:     page,
		PageSize: pageSize,
		Articles: []model.Article{},
	}
	if total := parseCount(doc.Find(selTotalCount).First().Text()); total >= 0 {
		result.Total = total
	}
	if mark := strings.TrimSpace(doc.Find(selPageMark).First().Text()); mark != "" {
		if _, after, ok := strings.Cut(mark, "/"); ok {
			result.PageCount = parseCount(after)
		}
	}

	offset := (page - 1) * pageSize
	doc.Find(".middle").Each(func(i int, block *goquery.Selection) {
		article := model.Article{Index: offset + i + 1}

		article.Sentence = normalizeText(block.Find("h5").First().Text())
		// h5 里带 "句子1：" 这类标签前缀, 去掉以便直接阅读.
		article.Sentence = strings.TrimPrefix(article.Sentence, "句子1：")
		article.Sentence = strings.TrimPrefix(article.Sentence, "句子2：")

		titleLink := block.Find("h6 a.fz14").First()
		if titleLink.Length() == 0 {
			titleLink = block.Find("h6 a").First()
		}
		article.Title = normalizeText(titleLink.Text())
		if href, ok := titleLink.Attr("href"); ok {
			article.DetailURL = absURL(href)
		}

		base := block.Find(".baseinfo").First()
		base.Find("a.KnowledgeNetLink").Each(func(_ int, a *goquery.Selection) {
			if name := strings.Trim(normalizeText(a.Text()), ";；"); name != "" {
				article.Authors = append(article.Authors, name)
			}
		})
		// 来源是 baseinfo 里不带 KnowledgeNetLink 类的那个链接.
		base.Find("span a").Each(func(_ int, a *goquery.Selection) {
			if a.HasClass("KnowledgeNetLink") {
				return
			}
			if article.Source == "" {
				article.Source = normalizeText(a.Text())
			}
		})
		if article.Source == "" {
			article.Source = normalizeText(base.Find("span").Last().Text())
		}
		article.Type = strings.Trim(normalizeText(base.Find("em").First().Text()), "【】")
		article.Date = normalizeText(base.Find(".date").First().Text())
		if count := parseCount(base.Find(".downloadCnt em").First().Text()); count > 0 {
			article.DownCount = count
		}

		if article.Title == "" && article.Sentence == "" {
			return
		}
		result.Articles = append(result.Articles, article)
	})

	return result, nil
}

// absURL 把相对地址补全为知网绝对地址.
func absURL(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return "https://kns.cnki.net" + path
}
