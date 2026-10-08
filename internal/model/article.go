// Package model 定义知网检索结果的领域模型.
//
// 这一层刻意与知网的 HTML 结构和请求参数解耦: 解析层负责把 HTML 变成
// 这里的结构, 输出层只认这里的结构. 因此知网改版时改动集中在解析层.
package model

import "strings"

// Article 是一篇文献的元数据.
//
// 字段分两类: 结果列表页就能拿到的, 以及只有详情页才有的. 后者在
// 只做列表检索时保持零值, 由 omitempty 在 JSON 输出里略去.
type Article struct {
	// Index 是该条在本次结果集中的序号, 从 1 开始.
	Index int `json:"index"`

	// 列表页字段.
	Title      string   `json:"title"`
	Authors    []string `json:"authors,omitempty"`
	Source     string   `json:"source,omitempty"`
	Date       string   `json:"date,omitempty"`
	Type       string   `json:"type,omitempty"`
	CitedCount int      `json:"cited_count,omitempty"`
	DownCount  int      `json:"download_count,omitempty"`
	HasFullTxt bool     `json:"has_full_text,omitempty"`

	// 定位标识, 用于拼装详情页地址与发起导出, 下载请求.
	Filename string `json:"filename,omitempty"`
	Dbcode   string `json:"dbcode,omitempty"`
	Dbname   string `json:"dbname,omitempty"`

	// DetailURL 是文献详情页地址.
	DetailURL string `json:"detail_url,omitempty"`

	// Sentence 是句子检索命中的原句, 只有句子检索会填充.
	Sentence string `json:"sentence,omitempty"`

	// 详情页字段.
	Abstract      string   `json:"abstract,omitempty"`
	Keywords      []string `json:"keywords,omitempty"`
	DOI           string   `json:"doi,omitempty"`
	Fund          string   `json:"fund,omitempty"`
	CLC           string   `json:"clc,omitempty"`
	Volume        string   `json:"volume,omitempty"`
	Issue         string   `json:"issue,omitempty"`
	Pages         string   `json:"pages,omitempty"`
	ISSN          string   `json:"issn,omitempty"`
	Organizations []string `json:"organizations,omitempty"`
}

// AuthorNames 把作者列表拼成便于展示的字符串.
func (a Article) AuthorNames() string {
	if len(a.Authors) == 0 {
		return ""
	}
	return strings.Join(a.Authors, "; ")
}

// Identifier 返回该文献的稳定标识, 优先使用 库代码:文件名 的组合.
//
// 知网的 filename 只在库内唯一, 因此标识必须带上 dbcode.
func (a Article) Identifier() string {
	if a.Filename == "" {
		return ""
	}
	if a.Dbcode == "" {
		return a.Filename
	}
	return a.Dbcode + ":" + a.Filename
}

// SearchResult 是一次检索的完整结果.
type SearchResult struct {
	// Query 是产生该结果的检索式描述, 便于复现与审计.
	Query string `json:"query,omitempty"`
	// Total 是知网报告的总命中数.
	Total int `json:"total"`
	// Page 是当前页码, 从 1 开始.
	Page int `json:"page"`
	// PageSize 是每页条数.
	PageSize int `json:"page_size"`
	// PageCount 是总页数.
	PageCount int `json:"page_count,omitempty"`
	// Articles 是当前页的文献列表.
	Articles []Article `json:"articles"`
	// Groups 是分组聚合数据, 仅在请求分组时填充.
	Groups []Group `json:"groups,omitempty"`
}

// Group 是知网结果页左侧的分组聚合项.
type Group struct {
	// Field 是分组维度, 例如 Subject / Year / Source / Organization.
	Field string `json:"field"`
	// Label 是该维度的中文名, 例如 学科 / 发表年度.
	Label string `json:"label,omitempty"`
	// Items 是该维度下的取值与命中数.
	Items []GroupItem `json:"items"`
}

// GroupItem 是分组下的一个取值.
type GroupItem struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// Entity 是导航检索返回的聚合实体 (作者, 机构, 基金, 期刊).
type Entity struct {
	// Kind 是实体类型, 取值为 author / organization / fund / journal.
	Kind string `json:"kind"`
	// Name 是实体名称.
	Name string `json:"name"`
	// Count 是发文量或相关文献量.
	Count int `json:"count,omitempty"`
	// Extra 存放该实体类型特有的补充字段, 例如期刊的 ISSN, 作者的机构.
	Extra map[string]string `json:"extra,omitempty"`
	// URL 是该实体的知网主页地址.
	URL string `json:"url,omitempty"`
}
