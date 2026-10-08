package output

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/azazo1/cnki-cli/internal/model"
)

// 知网不提供 RIS 与 BibTeX 导出, 这两种格式由本程序在本地从文献模型渲染.
//
// 数据完整度取决于来源: 结果列表页只含标题, 作者, 来源, 日期, 而卷期页码,
// DOI, 摘要等字段需要先取回详情页. 因此本地导出的字段完整度以实际拿到的
// 数据为准, 缺失字段会被省略而不是填入占位符.

// RenderRIS 渲染 RIS 格式题录.
func RenderRIS(w io.Writer, articles []model.Article) error {
	for _, a := range articles {
		if err := writeRISRecord(w, a); err != nil {
			return err
		}
	}
	return nil
}

// writeRISRecord 输出单条 RIS 记录.
func writeRISRecord(w io.Writer, a model.Article) error {
	lines := []string{}
	lines = append(lines, fmt.Sprintf("TY  - %s", risType(a)))
	for _, author := range a.Authors {
		lines = append(lines, fmt.Sprintf("AU  - %s", author))
	}
	lines = append(lines, fmt.Sprintf("TI  - %s", a.Title))
	if a.Source != "" {
		lines = append(lines, fmt.Sprintf("JO  - %s", a.Source))
	}
	if a.Abstract != "" {
		lines = append(lines, fmt.Sprintf("AB  - %s", a.Abstract))
	}
	for _, keyword := range a.Keywords {
		lines = append(lines, fmt.Sprintf("KW  - %s", keyword))
	}
	if year, month, day := splitDate(a.Date); year != "" {
		lines = append(lines, fmt.Sprintf("PY  - %s", year))
		// RIS 只应有一条 DA 记录, 能取到完整日期就用完整的.
		switch {
		case month != "" && day != "":
			lines = append(lines, fmt.Sprintf("DA  - %s/%s/%s", year, month, day))
		case month != "":
			lines = append(lines, fmt.Sprintf("DA  - %s/%s", year, month))
		}
	}
	if a.Volume != "" {
		lines = append(lines, fmt.Sprintf("VL  - %s", a.Volume))
	}
	if a.Issue != "" {
		lines = append(lines, fmt.Sprintf("IS  - %s", a.Issue))
	}
	if a.Pages != "" {
		lines = append(lines, fmt.Sprintf("SP  - %s", a.Pages))
	}
	if a.DOI != "" {
		lines = append(lines, fmt.Sprintf("DO  - %s", a.DOI))
	}
	if a.ISSN != "" {
		lines = append(lines, fmt.Sprintf("SN  - %s", a.ISSN))
	}
	if a.Fund != "" {
		lines = append(lines, fmt.Sprintf("A2  - %s", a.Fund))
	}
	if a.DetailURL != "" {
		lines = append(lines, fmt.Sprintf("UR  - %s", a.DetailURL))
	} else if a.Filename != "" {
		lines = append(lines, fmt.Sprintf("ID  - %s", a.Identifier()))
	}
	lines = append(lines, "ER  - ", "")

	for _, line := range lines {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}

// risType 把知网文献类型映射为 RIS 类型标识.
//
// Dbname 形如 "期刊", "博士", "硕士", "国内会议", "国际会议", "报纸",
// "中国专利", "国家标准". 名称里带关键字即归类.
func risType(a model.Article) string {
	name := a.Dbname + a.Type
	switch {
	case strings.Contains(name, "博士"), strings.Contains(name, "硕士"),
		strings.Contains(name, "学位"), strings.Contains(name, "Dissertation"):
		return "THES"
	case strings.Contains(name, "会议"), strings.Contains(name, "Conference"):
		return "CONF"
	case strings.Contains(name, "报纸"), strings.Contains(name, "Newspaper"):
		return "NEWS"
	case strings.Contains(name, "专利"), strings.Contains(name, "Patent"):
		return "PAT"
	case strings.Contains(name, "标准"), strings.Contains(name, "Standard"):
		return "STANDARD"
	case strings.Contains(name, "图书"), strings.Contains(name, "Book"):
		return "BOOK"
	case strings.Contains(name, "年鉴"), strings.Contains(name, "Almanac"):
		return "BOOK"
	case strings.Contains(name, "法律法规"), strings.Contains(name, "Law"):
		return "STAT"
	case strings.Contains(name, "成果"), strings.Contains(name, "报告"):
		return "RPRT"
	default:
		return "JOUR"
	}
}

// RenderBibTeX 渲染 BibTeX 格式题录.
func RenderBibTeX(w io.Writer, articles []model.Article) error {
	used := make(map[string]int, len(articles))
	for _, a := range articles {
		key := bibKey(a)
		if count, ok := used[key]; ok {
			count++
			used[key] = count
			key = fmt.Sprintf("%s-%c", key, 'a'+rune(count-1))
		} else {
			used[key] = 0
		}
		if err := writeBibEntry(w, key, a); err != nil {
			return err
		}
	}
	return nil
}

// writeBibEntry 输出单条 BibTeX 记录.
func writeBibEntry(w io.Writer, key string, a model.Article) error {
	if _, err := fmt.Fprintf(w, "@%s{%s,\n", bibType(a), key); err != nil {
		return err
	}
	fields := [][2]string{
		{"title", a.Title},
		{"author", strings.Join(a.Authors, " and ")},
		{"journal", a.Source},
		{"booktitle", a.Source},
		{"year", splitYear(a.Date)},
		{"volume", a.Volume},
		{"number", a.Issue},
		{"pages", a.Pages},
		{"doi", a.DOI},
		{"issn", a.ISSN},
		{"keywords", strings.Join(a.Keywords, ", ")},
		{"abstract", a.Abstract},
		{"note", a.Dbname},
		{"url", pickURL(a)},
	}
	kind := bibType(a)
	for _, field := range fields {
		name, value := field[0], field[1]
		// 期刊条目不用 booktitle, 会议条目不用 journal, 避免冗余字段.
		if value == "" {
			continue
		}
		if name == "journal" && kind != "article" {
			continue
		}
		if name == "booktitle" && kind == "article" {
			continue
		}
		if _, err := fmt.Fprintf(w, "  %s = {%s},\n", name, escapeBibTeX(value)); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(w, "}")
	return err
}

// bibType 把知网文献类型映射为 BibTeX 条目类型.
func bibType(a model.Article) string {
	name := a.Dbname + a.Type
	switch {
	case strings.Contains(name, "博士"):
		return "phdthesis"
	case strings.Contains(name, "硕士"):
		return "mastersthesis"
	case strings.Contains(name, "学位"), strings.Contains(name, "Dissertation"):
		return "phdthesis"
	case strings.Contains(name, "会议"), strings.Contains(name, "Conference"):
		return "inproceedings"
	case strings.Contains(name, "图书"), strings.Contains(name, "Book"):
		return "book"
	case strings.Contains(name, "标准"), strings.Contains(name, "专利"),
		strings.Contains(name, "报告"), strings.Contains(name, "成果"):
		return "techreport"
	case strings.Contains(name, "报纸"), strings.Contains(name, "Newspaper"):
		return "misc"
	default:
		return "article"
	}
}

// bibKey 生成 BibTeX 引用键.
//
// 引用键必须是 ASCII 且唯一. 中文作者的姓名与中文标题都无法直接用作键,
// 因此这里以"作者姓氏 (仅西文可直接采用) + 年份 + 标题哈希"构成:
// 中文文献得到形如 cnki2024a3f9c1 的键, 西文文献得到形如 smith2024a3f9c1
// 的键. 哈希保证唯一性, 避免同作者同年份的文献互相覆盖.
func bibKey(a model.Article) string {
	var b strings.Builder
	b.WriteString("cnki")
	if len(a.Authors) > 0 {
		if surname := sanitizeKey(latinize(a.Authors[0])); surname != "" && !isCJK(a.Authors[0]) {
			b.Reset()
			b.WriteString(strings.ToLower(surname))
		}
	}
	if year := splitYear(a.Date); year != "" {
		b.WriteString(year)
	}
	b.WriteString(shortHash(a.Title))
	return b.String()
}

// shortHash 返回文本的短哈希, 用于构造稳定的唯一后缀.
//
// 采用 FNV-1a: 无需引入依赖, 且对同一输入始终得到同一结果, 因此重复导出
// 同一批文献时引用键保持不变, 不会打断已有的 LaTeX 引用.
func shortHash(text string) string {
	if text == "" {
		return "000000"
	}
	const (
		offset = uint32(2166136261)
		prime  = uint32(16777619)
	)
	hash := offset
	for _, r := range text {
		hash ^= uint32(r)
		hash *= prime
	}
	const digits = "0123456789abcdef"
	out := make([]byte, 6)
	for i := 5; i >= 0; i-- {
		out[i] = digits[hash&0xF]
		hash >>= 4
	}
	return string(out)
}

// isCJK 判断文本是否含中日韩字符.
func isCJK(text string) bool {
	for _, r := range text {
		if r >= 0x2E80 {
			return true
		}
	}
	return false
}

// splitYear 从日期文本中取出年份.
func splitYear(date string) string {
	date = strings.TrimSpace(date)
	if len(date) >= 4 {
		candidate := date[:4]
		if isDigits(candidate) {
			return candidate
		}
	}
	return ""
}

// splitDate 把知网日期拆为年, 月, 日.
func splitDate(date string) (string, string, string) {
	parts := nonDigitSeparator.Split(strings.TrimSpace(date), -1)
	kept := make([]string, 0, 3)
	for _, part := range parts {
		if part != "" {
			kept = append(kept, part)
		}
	}
	for len(kept) < 3 {
		kept = append(kept, "")
	}
	return kept[0], kept[1], kept[2]
}

// nonDigitSeparator 用于按非数字字符切分日期.
var nonDigitSeparator = regexp.MustCompile(`[^0-9]+`)

// isDigits 判断字符串是否全为数字.
func isDigits(text string) bool {
	if text == "" {
		return false
	}
	for _, r := range text {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// firstTitleWord 取标题的第一个词, 用于构造引用键.
func firstTitleWord(title string) string {
	for _, r := range title {
		if r > 0x7F {
			// 中文标题不做拉丁化, 直接返回空, 由其他部分构成引用键.
			return ""
		}
		break
	}
	fields := strings.Fields(title)
	if len(fields) == 0 {
		return ""
	}
	word := fields[0]
	word = strings.Trim(word, ".,:;!?()[]{}")
	return word
}

// sanitizeKey 去掉引用键里不允许的字符.
func sanitizeKey(text string) string {
	var b strings.Builder
	for _, r := range text {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		}
	}
	return b.String()
}

// latinize 对非 ASCII 字符做简化处理, 中文作者名保留原样但会被 sanitizeKey 过滤.
func latinize(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	// 取最后一个空格分隔的片段作为姓氏, 符合西文姓名习惯.
	fields := strings.Fields(text)
	if len(fields) > 1 {
		return fields[len(fields)-1]
	}
	return text
}

// escapeBibTeX 转义 BibTeX 中的特殊字符.
func escapeBibTeX(text string) string {
	replacer := strings.NewReplacer(
		`\`, `\textbackslash{}`,
		`&`, `\&`,
		`%`, `\%`,
		`$`, `\$`,
		`#`, `\#`,
		`_`, `\_`,
		`{`, `\{`,
		`}`, `\}`,
	)
	return replacer.Replace(text)
}

// pickURL 选择最合适的链接.
func pickURL(a model.Article) string {
	if a.DetailURL != "" {
		return a.DetailURL
	}
	if a.DOI != "" {
		return "https://doi.org/" + a.DOI
	}
	return ""
}

// firstNonEmpty 返回首个非空值.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
