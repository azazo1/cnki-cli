// Package output 负责把领域模型渲染成用户可见的文本.
//
// stdout 只承载结果本身: 表格, JSON, CSV 或题录. 日志一律走 stderr,
// 这样 --json 的输出可以直接喂给 jq 而不被日志行打断.
package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/azazo1/cnki-cli/internal/apperr"
	"github.com/azazo1/cnki-cli/internal/model"
)

// 支持的输出格式.
const (
	FormatTable    = "table"
	FormatJSON     = "json"
	FormatCSV      = "csv"
	FormatMarkdown = "markdown"
)

// Formats 是全部可用的输出格式.
var Formats = []string{FormatTable, FormatJSON, FormatCSV, FormatMarkdown}

// ValidFormat 判断格式名是否受支持.
func ValidFormat(format string) bool {
	for _, f := range Formats {
		if strings.EqualFold(f, format) {
			return true
		}
	}
	return false
}

// RenderSearchResult 渲染一次检索结果.
func RenderSearchResult(w io.Writer, format string, result *model.SearchResult, limit int) error {
	if result == nil {
		return apperr.New(apperr.ExitFailure, "结果为空")
	}
	articles := result.Articles
	if limit > 0 && len(articles) > limit {
		articles = articles[:limit]
	}

	switch strings.ToLower(format) {
	case FormatJSON:
		return renderSearchJSON(w, result, articles)
	case FormatCSV:
		return renderArticlesCSV(w, articles)
	case FormatMarkdown:
		return renderArticlesMarkdown(w, articles)
	default:
		return renderArticlesTable(w, articles, result)
	}
}

// RenderArticles 渲染一组不带检索上下文的文献.
func RenderArticles(w io.Writer, format string, articles []model.Article) error {
	switch strings.ToLower(format) {
	case FormatJSON:
		return writeJSON(w, articles)
	case FormatCSV:
		return renderArticlesCSV(w, articles)
	case FormatMarkdown:
		return renderArticlesMarkdown(w, articles)
	default:
		return renderArticlesTable(w, articles, nil)
	}
}

// RenderGroups 渲染分组聚合结果.
func RenderGroups(w io.Writer, format string, groups []model.Group) error {
	switch strings.ToLower(format) {
	case FormatJSON:
		return writeJSON(w, groups)
	default:
		for _, g := range groups {
			label := g.Label
			if label == "" {
				label = g.Field
			}
			if _, err := fmt.Fprintf(w, "%s:\n", label); err != nil {
				return err
			}
			for _, item := range g.Items {
				if _, err := fmt.Fprintf(w, "  %-28s %6d\n", item.Value, item.Count); err != nil {
					return err
				}
			}
		}
		return nil
	}
}

// RenderEntities 渲染导航检索返回的实体列表.
func RenderEntities(w io.Writer, format string, entities []model.Entity) error {
	switch strings.ToLower(format) {
	case FormatJSON:
		return writeJSON(w, entities)
	case FormatCSV, FormatMarkdown:
		rows := make([][]string, 0, len(entities))
		for _, e := range entities {
			rows = append(rows, []string{e.Name, fmt.Sprintf("%d", e.Count), e.URL})
		}
		header := []string{"名称", "数量", "地址"}
		if strings.EqualFold(format, FormatCSV) {
			return writeCSV(w, header, rows)
		}
		return writeMarkdown(w, header, rows)
	default:
		rows := make([][]string, 0, len(entities))
		for _, e := range entities {
			rows = append(rows, []string{e.Name, fmt.Sprintf("%d", e.Count)})
		}
		return writeTable(w, []string{"名称", "数量"}, rows)
	}
}
