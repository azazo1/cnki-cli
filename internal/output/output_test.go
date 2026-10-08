package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/azazo1/cnki-cli/internal/model"
)

func sampleArticles() []model.Article {
	return []model.Article{
		{
			Index:      1,
			Title:      "深度学习在医学影像中的应用",
			Authors:    []string{"张三", "李四"},
			Source:     "计算机学报",
			Date:       "2024-03-15",
			Dbname:     "期刊",
			Keywords:   []string{"深度学习", "医学影像"},
			DOI:        "10.1234/abcd",
			DetailURL:  "https://kns.cnki.net/kcms2/article/abstract?v=abc",
			CitedCount: 12,
			DownCount:  345,
		},
		{
			Index:   2,
			Title:   "某博士学位论文",
			Authors: []string{"王五"},
			Source:  "某某大学",
			Date:    "2023",
			Dbname:  "博士",
		},
	}
}

func TestRenderRIS(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderRIS(&buf, sampleArticles()); err != nil {
		t.Fatalf("渲染失败: %v", err)
	}
	got := buf.String()

	// 期刊条目应为 JOUR, 学位论文应为 THES.
	if !strings.Contains(got, "TY  - JOUR") {
		t.Error("期刊未映射为 JOUR")
	}
	if !strings.Contains(got, "TY  - THES") {
		t.Error("学位论文未映射为 THES")
	}
	// 每条记录必须以 ER 终止, 否则多数文献管理软件会解析失败.
	if count := strings.Count(got, "ER  - "); count != 2 {
		t.Errorf("ER 终止行数量错误: 期望 2, 实际 %d", count)
	}
	if !strings.Contains(got, "AU  - 张三") {
		t.Error("作者未输出")
	}
	if !strings.Contains(got, "DO  - 10.1234/abcd") {
		t.Error("DOI 未输出")
	}
	// 年份应单独成行, 且不能被截断.
	if !strings.Contains(got, "PY  - 2024") {
		t.Error("年份输出错误")
	}
}

func TestRenderBibTeX(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderBibTeX(&buf, sampleArticles()); err != nil {
		t.Fatalf("渲染失败: %v", err)
	}
	got := buf.String()

	if !strings.Contains(got, "@article{") {
		t.Error("期刊未映射为 article")
	}
	if !strings.Contains(got, "@phdthesis{") {
		t.Error("博士论文未映射为 phdthesis")
	}
	// 作者之间应使用 BibTeX 的 and 连接.
	if !strings.Contains(got, "author = {张三 and 李四}") {
		t.Errorf("作者格式错误:\n%s", got)
	}
	if !strings.Contains(got, "title = {深度学习在医学影像中的应用}") {
		t.Error("标题未输出")
	}
	// 条目必须闭合.
	if strings.Count(got, "}") < 2 || !strings.HasSuffix(strings.TrimSpace(got), "}") {
		t.Errorf("条目未正确闭合:\n%s", got)
	}
	// 会议与期刊字段不应同时出现.
	if strings.Contains(got, "@article{") {
		for _, block := range strings.Split(got, "@article{")[1:] {
			if idx := strings.Index(block, "}"); idx > 0 {
				if strings.Contains(block[:idx], "booktitle") {
					t.Error("article 条目不应包含 booktitle")
				}
			}
		}
	}
}

func TestRenderBibTeXEscapesSpecialChars(t *testing.T) {
	var buf bytes.Buffer
	articles := []model.Article{{
		Title:   "关于 A&B 的 100% 研究",
		Authors: []string{"张三"},
		Date:    "2024",
		Dbname:  "期刊",
	}}
	if err := RenderBibTeX(&buf, articles); err != nil {
		t.Fatalf("渲染失败: %v", err)
	}
	got := buf.String()
	// 未转义的 & 与 % 在 LaTeX 中会报错.
	if strings.Contains(got, "A&B") {
		t.Error("& 未转义")
	}
	if strings.Contains(got, "100%") {
		t.Error("% 未转义")
	}
}

func TestDisplayWidth(t *testing.T) {
	// 中文按两格计算, 表格才能对齐.
	if got := displayWidth("中文"); got != 4 {
		t.Errorf("中文宽度应为 4, 实际 %d", got)
	}
	if got := displayWidth("abcd"); got != 4 {
		t.Errorf("英文宽度应为 4, 实际 %d", got)
	}
	if got := displayWidth("中a"); got != 3 {
		t.Errorf("混排宽度应为 3, 实际 %d", got)
	}
}

func TestTruncateRespectsDisplayWidth(t *testing.T) {
	text := strings.Repeat("中", 40)
	got := truncate(text, 20)
	if displayWidth(got) > 20 {
		t.Errorf("截断后宽度超限: %d", displayWidth(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("截断后应有省略号: %q", got)
	}

	// 未超限时不应改动原文.
	if got := truncate("短文本", 20); got != "短文本" {
		t.Errorf("未超限时不应改动: %q", got)
	}
}

func TestWriteTableAlignsColumns(t *testing.T) {
	var buf bytes.Buffer
	err := writeTable(&buf, []string{"列一", "列二"}, [][]string{
		{"a", "中"},
		{"长内容", "b"},
	})
	if err != nil {
		t.Fatalf("渲染失败: %v", err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("应有表头, 分隔线, 两行数据, 实际 %d 行", len(lines))
	}

	// 表头与分隔线都不含尾随空白, 两者必须等宽.
	if displayWidth(lines[0]) != displayWidth(lines[1]) {
		t.Errorf("表头宽度 %d 与分隔线宽度 %d 不一致",
			displayWidth(lines[0]), displayWidth(lines[1]))
	}
	// 数据行会裁掉行尾空白, 因此宽度不超过表头宽度, 且第二列起点必须对齐.
	secondColumnStart := displayWidth("长内容") + 2
	for i, line := range lines[2:] {
		if displayWidth(line) > displayWidth(lines[0]) {
			t.Errorf("第 %d 行数据宽度超出表头", i)
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		// 第二列内容应恰好落在表头定义的列位置上.
		idx := strings.Index(line, "b")
		if idx < 0 {
			continue
		}
		if got := displayWidth(line[:idx]); got != secondColumnStart {
			t.Errorf("第二列起点应为 %d, 实际 %d", secondColumnStart, got)
		}
	}
}

func TestRenderSearchResultJSON(t *testing.T) {
	var buf bytes.Buffer
	result := &model.SearchResult{
		Query:    "主题=测试",
		Total:    100,
		Page:     1,
		PageSize: 20,
		Articles: sampleArticles(),
	}
	if err := RenderSearchResult(&buf, FormatJSON, result, 1); err != nil {
		t.Fatalf("渲染失败: %v", err)
	}
	got := buf.String()
	// total 应反映知网报告的真实命中量, returned 反映本次实际输出条数.
	if !strings.Contains(got, `"total": 100`) {
		t.Errorf("total 字段错误:\n%s", got)
	}
	if !strings.Contains(got, `"returned": 1`) {
		t.Errorf("returned 字段未反映裁剪结果:\n%s", got)
	}
}

func TestRenderSearchResultEmpty(t *testing.T) {
	var buf bytes.Buffer
	result := &model.SearchResult{Total: 0, Articles: []model.Article{}}
	if err := RenderSearchResult(&buf, FormatTable, result, 0); err != nil {
		t.Fatalf("空结果不应报错: %v", err)
	}
	if !strings.Contains(buf.String(), "没有匹配结果") {
		t.Errorf("空结果提示缺失: %q", buf.String())
	}
}

func TestValidFormat(t *testing.T) {
	for _, f := range Formats {
		if !ValidFormat(f) {
			t.Errorf("格式 %s 应被支持", f)
		}
	}
	if ValidFormat("不存在的格式") {
		t.Error("未知格式不应被接受")
	}
}
