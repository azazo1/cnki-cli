package cnki

import (
	"context"
	"fmt"

	"github.com/azazo1/cnki-cli/internal/apperr"
	"github.com/azazo1/cnki-cli/internal/model"
	"github.com/azazo1/cnki-cli/internal/taxonomy"
)

// 知网结果集的相关限制.
const (
	// PathBriefGrid 是检索接口地址.
	PathBriefGrid = "/kns8s/brief/grid"
	// PathSentence 是句子检索接口地址.
	PathSentence = "/kns8s/brief/senquery"
	// MaxResultPages 是知网允许翻页的最大页数.
	//
	// 结果页会出现 "1/300" 这样的标记, 超过部分知网不再返回.
	MaxResultPages = 300
	// MaxPageSize 是知网允许的每页最大条数.
	MaxPageSize = 50
)

// Search 执行一次检索并解析结果.
//
// 检索式自带的页码与每页条数会被使用, 调用方通过 Query.SetPage 翻页.
func (c *Client) Search(ctx context.Context, q *Query) (*model.SearchResult, error) {
	if q == nil {
		return nil, apperr.Usage("检索式为空")
	}
	opts := q.opts.withDefaults()
	if opts.PageSize > MaxPageSize {
		return nil, apperr.Usage("每页条数最大为 %d, 当前为 %d", MaxPageSize, opts.PageSize)
	}
	if opts.Page > MaxResultPages {
		return nil, apperr.Usage("知网最多提供前 %d 页结果, 请求的页码为 %d", MaxResultPages, opts.Page)
	}

	form, err := q.Form()
	if err != nil {
		return nil, err
	}

	path := PathBriefGrid
	if q.searchType == taxonomy.SearchSentence {
		path = PathSentence
	}

	resp, err := c.PostForm(ctx, path, form)
	if err != nil {
		return nil, err
	}

	// 句子检索的结果页没有结果表格, 需要走专门的解析器.
	parse := ParseSearchResult
	if q.searchType == taxonomy.SearchSentence {
		parse = ParseSentenceResult
	}
	result, err := parse(resp.Text(), opts.Page, opts.PageSize)
	if err != nil {
		return nil, err
	}
	result.Query = q.Description()
	return result, nil
}

// SearchPages 连续取回多页结果.
//
// limit 是期望取回的记录总数, 0 表示不限制 (此时以知网的页数上限为界).
// 每页之间会自动遵守客户端的请求间隔.
func (c *Client) SearchPages(ctx context.Context, q *Query, limit int) (*model.SearchResult, error) {
	opts := q.opts.withDefaults()
	merged := &model.SearchResult{
		Page:     1,
		PageSize: opts.PageSize,
		Articles: []model.Article{},
	}

	page := opts.Page
	for {
		select {
		case <-ctx.Done():
			return merged, apperr.Wrap(apperr.ExitInterrupt, ctx.Err(), "检索被中断")
		default:
		}

		q.SetPage(page)
		current, err := c.Search(ctx, q)
		if err != nil {
			return merged, err
		}

		merged.Total = current.Total
		merged.PageCount = current.PageCount
		merged.Query = current.Query
		merged.Articles = append(merged.Articles, current.Articles...)

		if limit > 0 && len(merged.Articles) >= limit {
			merged.Articles = merged.Articles[:limit]
			break
		}
		if len(current.Articles) == 0 {
			break
		}
		if current.PageCount > 0 && page >= current.PageCount {
			break
		}
		if page >= MaxResultPages {
			break
		}
		if limit <= 0 && current.PageCount == 0 {
			// 没有明确页数且未指定上限时, 以本页条数不足一页作为结束标志.
			if len(current.Articles) < opts.PageSize {
				break
			}
		}
		page++

		if c.logger != nil {
			c.logger.Debug("翻页完成", "page", page-1, "累计", len(merged.Articles), "总数", merged.Total)
		}
	}

	return merged, nil
}

// ResultHint 生成一行结果统计说明, 用于 CLI 展示知网结果集边界.
func ResultHint(r *model.SearchResult) string {
	if r == nil {
		return ""
	}
	text := fmt.Sprintf("共 %d 条", r.Total)
	if r.PageCount > 0 {
		text += fmt.Sprintf(", 共 %d 页", r.PageCount)
	}
	if r.PageCount >= MaxResultPages {
		text += fmt.Sprintf(" (知网仅开放前 %d 页)", MaxResultPages)
	}
	return text
}
