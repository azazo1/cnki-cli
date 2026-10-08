package output

import (
	"encoding/csv"
	"encoding/json"
	"io"
	"strconv"

	"github.com/azazo1/cnki-cli/internal/apperr"
	"github.com/azazo1/cnki-cli/internal/model"
)

// itoa 是 strconv.Itoa 的短别名, 避免在列构造处反复书写包名.
func itoa(value int) string { return strconv.Itoa(value) }

// writeJSON 以缩进 JSON 输出任意结构.
func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return apperr.Wrap(apperr.ExitFailure, err, "输出 JSON")
	}
	return nil
}

// renderSearchJSON 输出检索结果.
//
// limit 用于裁剪条数, 但保留 total 字段反映知网报告的真实命中量.
func renderSearchJSON(w io.Writer, result *model.SearchResult, articles []model.Article) error {
	payload := struct {
		Query     string          `json:"query,omitempty"`
		Total     int             `json:"total"`
		Page      int             `json:"page"`
		PageSize  int             `json:"page_size"`
		PageCount int             `json:"page_count,omitempty"`
		Returned  int             `json:"returned"`
		Articles  []model.Article `json:"articles"`
		Groups    []model.Group   `json:"groups,omitempty"`
	}{
		Query:     result.Query,
		Total:     result.Total,
		Page:      result.Page,
		PageSize:  result.PageSize,
		PageCount: result.PageCount,
		Returned:  len(articles),
		Articles:  articles,
		Groups:    result.Groups,
	}
	if payload.Articles == nil {
		payload.Articles = []model.Article{}
	}
	return writeJSON(w, payload)
}

// csvHeader 是文献 CSV 的列名.
var csvHeader = []string{
	"序号", "题名", "作者", "来源", "日期", "数据库", "被引", "下载", "链接",
}

// renderArticlesCSV 以 CSV 渲染文献列表.
func renderArticlesCSV(w io.Writer, articles []model.Article) error {
	rows := make([][]string, 0, len(articles))
	for _, a := range articles {
		rows = append(rows, []string{
			itoa(a.Index),
			a.Title,
			a.AuthorNames(),
			a.Source,
			a.Date,
			a.Dbname,
			itoa(a.CitedCount),
			itoa(a.DownCount),
			a.DetailURL,
		})
	}
	return writeCSV(w, csvHeader, rows)
}

// writeCSV 输出 CSV, 带 UTF-8 BOM 以便 Excel 正确识别中文.
func writeCSV(w io.Writer, header []string, rows [][]string) error {
	if _, err := w.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return err
	}
	writer := csv.NewWriter(w)
	if err := writer.Write(header); err != nil {
		return err
	}
	for _, row := range rows {
		if err := writer.Write(row); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}
