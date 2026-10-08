package cnki

import (
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
		// 空结果时知网只返回一句提示, 这属于正常情况而非错误.
		if doc.Find(selNoContent).Length() > 0 {
			return result, nil
		}
		return nil, apperr.Remote("结果页中未找到结果表格, 知网可能已改版")
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
