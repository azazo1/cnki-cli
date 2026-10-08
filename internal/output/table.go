package output

import (
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/azazo1/cnki-cli/internal/model"
)

// renderArticlesTable 以等宽表格渲染文献列表.
func renderArticlesTable(w io.Writer, articles []model.Article, result *model.SearchResult) error {
	if len(articles) == 0 {
		_, err := fmt.Fprintln(w, "没有匹配结果")
		return err
	}
	if result != nil {
		if _, err := fmt.Fprintf(w, "%s\n\n", resultSummary(result, len(articles))); err != nil {
			return err
		}
	}
	// 句子检索的结果以命中原句为主角, 用另一套列展示.
	if hasSentences(articles) {
		rows := make([][]string, 0, len(articles))
		for _, a := range articles {
			rows = append(rows, []string{
				fmt.Sprintf("%d", a.Index),
				truncate(a.Sentence, 66),
				truncate(a.Title, 34),
				truncate(a.AuthorNames(), 20),
				truncate(a.Date, 10),
			})
		}
		return writeTable(w, []string{"#", "命中句子", "出处", "作者", "日期"}, rows)
	}
	rows := make([][]string, 0, len(articles))
	for _, a := range articles {
		rows = append(rows, []string{
			fmt.Sprintf("%d", a.Index),
			truncate(a.Title, 60),
			truncate(a.AuthorNames(), 24),
			truncate(a.Source, 22),
			a.Date,
			a.Dbname,
			intOrDash(a.CitedCount),
			intOrDash(a.DownCount),
		})
	}
	header := []string{"#", "题名", "作者", "来源", "日期", "库", "被引", "下载"}
	return writeTable(w, header, rows)
}

// hasSentences 判断结果是否来自句子检索.
func hasSentences(articles []model.Article) bool {
	for _, a := range articles {
		if a.Sentence != "" {
			return true
		}
	}
	return false
}

// resultSummary 生成结果统计行.
func resultSummary(result *model.SearchResult, shown int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "共 %d 条", result.Total)
	if result.PageCount > 0 {
		fmt.Fprintf(&b, ", %d 页", result.PageCount)
	}
	fmt.Fprintf(&b, "; 本次输出 %d 条", shown)
	if result.Query != "" {
		fmt.Fprintf(&b, "; 检索式 %s", result.Query)
	}
	return b.String()
}

// renderArticlesMarkdown 以 Markdown 表格渲染文献列表.
func renderArticlesMarkdown(w io.Writer, articles []model.Article) error {
	if len(articles) == 0 {
		_, err := fmt.Fprintln(w, "没有匹配结果")
		return err
	}
	rows := make([][]string, 0, len(articles))
	for _, a := range articles {
		title := a.Title
		if a.DetailURL != "" {
			title = fmt.Sprintf("[%s](%s)", escapeMarkdown(title), a.DetailURL)
		}
		rows = append(rows, []string{
			fmt.Sprintf("%d", a.Index),
			title,
			a.AuthorNames(),
			a.Source,
			a.Date,
			a.Dbname,
			intOrDash(a.CitedCount),
			intOrDash(a.DownCount),
		})
	}
	return writeMarkdown(w, []string{"#", "题名", "作者", "来源", "日期", "库", "被引", "下载"}, rows)
}

// writeTable 渲染等宽字符表格.
//
// 宽度按东亚字符占两格的惯例计算, 保证中文列能对齐.
func writeTable(w io.Writer, header []string, rows [][]string) error {
	widths := make([]int, len(header))
	for i, h := range header {
		widths[i] = displayWidth(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i >= len(widths) {
				break
			}
			if width := displayWidth(cell); width > widths[i] {
				widths[i] = width
			}
		}
	}

	if err := writeTableRow(w, header, widths); err != nil {
		return err
	}
	separators := make([]string, len(header))
	for i := range separators {
		separators[i] = strings.Repeat("-", widths[i])
	}
	if err := writeTableRow(w, separators, widths); err != nil {
		return err
	}
	for _, row := range rows {
		if err := writeTableRow(w, row, widths); err != nil {
			return err
		}
	}
	return nil
}

func writeTableRow(w io.Writer, cells []string, widths []int) error {
	var b strings.Builder
	for i, width := range widths {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		b.WriteString(cell)
		if padding := width - displayWidth(cell); padding > 0 {
			b.WriteString(strings.Repeat(" ", padding))
		}
		if i < len(widths)-1 {
			b.WriteString("  ")
		}
	}
	_, err := fmt.Fprintln(w, strings.TrimRight(b.String(), " "))
	return err
}

// writeMarkdown 渲染 Markdown 表格.
func writeMarkdown(w io.Writer, header []string, rows [][]string) error {
	if _, err := fmt.Fprintf(w, "| %s |\n", strings.Join(header, " | ")); err != nil {
		return err
	}
	divider := make([]string, len(header))
	for i := range divider {
		divider[i] = "---"
	}
	if _, err := fmt.Fprintf(w, "| %s |\n", strings.Join(divider, " | ")); err != nil {
		return err
	}
	for _, row := range rows {
		cells := make([]string, len(header))
		for i := range header {
			if i < len(row) {
				cells[i] = strings.ReplaceAll(row[i], "|", "\\|")
			}
		}
		if _, err := fmt.Fprintf(w, "| %s |\n", strings.Join(cells, " | ")); err != nil {
			return err
		}
	}
	return nil
}

// displayWidth 估算字符串的显示宽度, 东亚宽字符与全角符号按两格计算.
func displayWidth(text string) int {
	width := 0
	for _, r := range text {
		width += runeWidth(r)
	}
	return width
}

// runeWidth 返回单个字符的显示宽度.
func runeWidth(r rune) int {
	if r == 0 {
		return 0
	}
	if r < 0x20 {
		return 0
	}
	if isWide(r) {
		return 2
	}
	return 1
}

// isWide 判断字符是否占据两格.
func isWide(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115F, // 谚文字母
		r >= 0x2E80 && r <= 0x303E, // 部首与符号
		r >= 0x3041 && r <= 0x33FF, // 假名, 注音, 兼容字符
		r >= 0x3400 && r <= 0x4DBF, // 扩展 A
		r >= 0x4E00 && r <= 0x9FFF, // 基本汉字
		r >= 0xA000 && r <= 0xA4CF, // 彝文
		r >= 0xAC00 && r <= 0xD7A3, // 谚文音节
		r >= 0xF900 && r <= 0xFAFF, // 兼容汉字
		r >= 0xFE30 && r <= 0xFE6F, // 兼容形式
		r >= 0xFF00 && r <= 0xFF60, // 全角形式
		r >= 0xFFE0 && r <= 0xFFE6,
		r >= 0x20000 && r <= 0x2FFFD,
		r >= 0x30000 && r <= 0x3FFFD:
		return true
	}
	return false
}

// truncate 按显示宽度截断文本, 超出部分以省略号结尾.
func truncate(text string, max int) string {
	if max <= 0 || displayWidth(text) <= max {
		return text
	}
	var b strings.Builder
	width := 0
	for _, r := range text {
		w := runeWidth(r)
		// 预留三个半角字符的位置给省略号.
		if width+w > max-3 {
			break
		}
		b.WriteRune(r)
		width += w
	}
	return b.String() + "..."
}

// escapeMarkdown 转义 Markdown 表格中的竖线.
func escapeMarkdown(text string) string {
	return strings.ReplaceAll(text, "|", "\\|")
}

// intOrDash 把 0 渲染为短横线, 避免表格里大片 0 干扰阅读.
func intOrDash(value int) string {
	if value == 0 {
		return "-"
	}
	return fmt.Sprintf("%d", value)
}

var _ = unicode.MaxRune
