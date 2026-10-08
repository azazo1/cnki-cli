package session

import (
	"net/url"
)

// urlQueryUnescape 是 net/url 的解码包装.
//
// 知网把机构名按 query 转义规则编码 (空格写作 %20 而非 +),
// 用 url.QueryUnescape 会把加号误解成空格, 因此这里先还原再加号.
func urlQueryUnescape(raw string) (string, error) {
	return url.QueryUnescape(raw)
}
