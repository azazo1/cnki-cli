package cnki

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/azazo1/cnki-cli/internal/apperr"
)

// DownloadFormat 是可用于下载的全文格式.
//
// 知网详情页还提供"原版阅读"与"HTML阅读", 但那两个是在线阅读入口而不是
// 文件下载, 因此不在此列.
type DownloadFormat string

const (
	// FormatPDF 是 PDF 全文.
	FormatPDF DownloadFormat = "pdf"
	// FormatCAJ 是知网自有阅读器格式, 保留原始版式, 需要 CAJViewer 打开.
	FormatCAJ DownloadFormat = "caj"
)

// downloadLabels 是详情页上最常见的下载项文案.
var downloadLabels = map[DownloadFormat]string{
	FormatPDF: "PDF下载",
	FormatCAJ: "CAJ下载",
}

// downloadMatcher 描述如何在详情页上认出一个格式的下载入口.
type downloadMatcher struct {
	// text 是文案里出现的关键字.
	//
	// 用关键字而不是完整文案: 知网在不同文献类型下的叫法不一致, 例如专利
	// 写的是 "CAJ原文下载", 期刊写的是 "CAJ下载".
	text string
	// parentClass 是父元素的类名特征, 作为文案匹配之外的兜底.
	parentClass string
}

// downloadMatchers 把格式映射到识别规则.
var downloadMatchers = map[DownloadFormat]downloadMatcher{
	FormatPDF: {text: "PDF", parentClass: "btn-dlpdf"},
	FormatCAJ: {text: "CAJ", parentClass: "btn-dlcaj"},
}

// ParseDownloadFormat 解析命令行给出的格式.
func ParseDownloadFormat(text string) (DownloadFormat, error) {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "pdf", "":
		return FormatPDF, nil
	case "caj":
		return FormatCAJ, nil
	default:
		return "", apperr.Usage("未知的全文格式 %q, 可用值: pdf, caj", text)
	}
}

// Label 返回该格式在知网界面上的下载项文案, 用于在 HTML 里定位入口.
func (f DownloadFormat) Label() string {
	if label, ok := downloadLabels[f]; ok {
		return label
	}
	return string(f)
}

// Name 返回该格式的简短名称, 用于给用户看的消息.
func (f DownloadFormat) Name() string {
	return strings.ToUpper(string(f))
}

// loginURLMark 是知网登录服务在地址里的特征.
const loginURLMark = "login.cnki.net"

// ErrLoginRequired 表示知网要求先登录或有订阅权限才能取全文.
//
// 实测未登录时下载下单地址会 302 到 login.cnki.net, 因此这不是错误猜测,
// 而是知网明确的拒绝方式.
var ErrLoginRequired = errors.New("知网要求先登录才能下载全文")

// IsLoginRequired 判断错误是否表示缺少下载权限.
func IsLoginRequired(err error) bool { return errors.Is(err, ErrLoginRequired) }

// maxDownloadHops 是下载下单允许的最大跳转次数.
//
// 实测链路是 下单地址 → (登录页 或 真实文件地址), 正常情况下不会超过两次;
// 给出余量既能容纳知网调整, 也能在异常跳转环上及时收手.
const maxDownloadHops = 5

// DownloadResult 是一次全文下载的结果.
type DownloadResult struct {
	// Data 是文件内容.
	Data []byte
	// Format 是识别出的文件格式.
	Format DownloadFormat
	// FileName 是建议的文件名, 可能为空.
	FileName string
}

// ParseDownloadOrderURL 从详情页里取出指定格式的下载下单地址.
//
// 详情页把这四种入口都列在 a[href*=bar/download/order] 上, 靠文案区分,
// 因此按文案匹配而不是按位置取.
func ParseDownloadOrderURL(html string, format DownloadFormat) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return "", apperr.Wrap(apperr.ExitFailure, err, "解析详情页 HTML")
	}

	matcher, ok := downloadMatchers[format]
	if !ok {
		return "", apperr.Usage("不支持下载 %s 格式", format.Name())
	}

	target := ""
	doc.Find("a[href*='bar/download/order']").EachWithBreak(func(_ int, a *goquery.Selection) bool {
		if !matchesDownload(a, matcher) {
			return true
		}
		href, ok := a.Attr("href")
		if !ok {
			return true
		}
		target = strings.TrimSpace(href)
		return false
	})

	if target == "" {
		return "", apperr.New(apperr.ExitFailure,
			"这篇文献没有提供 %s 全文, 可能只收录了其它格式. "+
				"用 cnki detail 可以看到该文献实际提供哪些格式", format.Name())
	}
	return target, nil
}

// matchesDownload 判断一个下载入口是否属于目标格式.
func matchesDownload(a *goquery.Selection, matcher downloadMatcher) bool {
	if strings.Contains(strings.ToUpper(normalizeText(a.Text())), matcher.text) {
		return true
	}
	parentClass, _ := a.Parent().Attr("class")
	return strings.Contains(strings.ToLower(parentClass), matcher.parentClass)
}

// DownloadFullText 取回全文文件.
//
// orderURL 是详情页给出的下载下单地址. 知网不会直接返回文件, 而是先跳转:
// 有权限时跳到真实文件地址, 没有权限时跳到登录页. 这里跟随跳转并识别后者,
// 以免把登录页当成文件存下来.
func (c *Client) DownloadFullText(ctx context.Context, orderURL string, referer string) (*DownloadResult, error) {
	current, err := c.normalizeOrderURL(orderURL)
	if err != nil {
		return nil, err
	}

	for hop := 0; hop < maxDownloadHops; hop++ {
		resp, err := c.getWithReferer(ctx, current, referer)
		if err != nil {
			return nil, err
		}

		location := strings.TrimSpace(resp.Header.Get("Location"))
		if location == "" {
			return classifyDownload(resp)
		}

		next, err := resolveRedirect(current, location)
		if err != nil {
			return nil, err
		}
		if strings.Contains(next, loginURLMark) {
			return nil, ErrLoginRequired
		}
		current = next
	}

	return nil, apperr.Remote("下载跳转次数超过 %d 次, 已放弃", maxDownloadHops)
}

// normalizeOrderURL 把下单地址补成绝对地址.
func (c *Client) normalizeOrderURL(orderURL string) (string, error) {
	orderURL = strings.TrimSpace(orderURL)
	if orderURL == "" {
		return "", apperr.Usage("下载地址为空")
	}
	if strings.HasPrefix(orderURL, "//") {
		return "https:" + orderURL, nil
	}
	if strings.HasPrefix(orderURL, "http://") || strings.HasPrefix(orderURL, "https://") {
		return orderURL, nil
	}
	return c.AbsURL(orderURL), nil
}

// resolveRedirect 相对当前地址解析跳转目标.
//
// 知网在 Location 里用协议相对地址 (//host/path), 直接用 url.Parse 再拼
// 会得到错误结果, 因此统一交给 net/url 的解析规则处理.
func resolveRedirect(current string, location string) (string, error) {
	base, err := url.Parse(current)
	if err != nil {
		return "", apperr.Wrap(apperr.ExitFailure, err, "解析当前地址 %s", current)
	}
	target, err := url.Parse(strings.TrimSpace(location))
	if err != nil {
		return "", apperr.Wrap(apperr.ExitFailure, err, "解析跳转地址 %s", location)
	}
	return base.ResolveReference(target).String(), nil
}

// classifyDownload 依据响应头与文件头判定下载结果.
//
// 优先看文件头: 知网返回的文件名与实际格式未必一致, 而文件头是硬证据.
func classifyDownload(resp *Response) (*DownloadResult, error) {
	// 没有权限时知网也可能直接把登录页以 200 返回, 而不是跳转, 因此这里也要
	// 认一次, 否则登录页会被当成未知格式, 报出与真实原因无关的错误.
	if isLoginPage(resp.Text()) {
		return nil, ErrLoginRequired
	}

	name := filenameFromDisposition(resp.Header.Get("Content-Disposition"))

	format := formatFromMagic(resp.Body)
	if format == "" {
		format = formatFromExtension(filepath.Ext(name))
	}
	if format == "" {
		return nil, apperr.Remote(
			"下载响应不是可识别的全文文件: 内容类型 %q, 文件名 %q, 开头字节 %s",
			resp.Header.Get("Content-Type"), name, previewBytes(resp.Body, 8))
	}
	if name == "" {
		name = "cnki-fulltext." + string(format)
	}
	return &DownloadResult{Data: resp.Body, Format: format, FileName: name}, nil
}

// formatFromMagic 按文件头判断格式, 无法识别时返回空串.
func formatFromMagic(data []byte) DownloadFormat {
	switch {
	case len(data) >= 4 && string(data[:4]) == "%PDF":
		return FormatPDF
	case len(data) >= 3 && string(data[:3]) == "CAJ":
		return FormatCAJ
	default:
		return ""
	}
}

// formatFromExtension 按扩展名判断格式.
func formatFromExtension(ext string) DownloadFormat {
	switch strings.ToLower(strings.TrimSpace(ext)) {
	case ".pdf":
		return FormatPDF
	case ".caj":
		return FormatCAJ
	default:
		return ""
	}
}

// filenameFromDisposition 从 Content-Disposition 里取出文件名.
//
// 兼容 filename="x" 与 RFC 5987 的 filename*=UTF-8”x 两种写法, 后者在
// 中文文件名上更常见.
func filenameFromDisposition(disposition string) string {
	disposition = strings.TrimSpace(disposition)
	if disposition == "" {
		return ""
	}

	for _, part := range strings.Split(disposition, ";") {
		part = strings.TrimSpace(part)
		lower := strings.ToLower(part)
		if !strings.HasPrefix(lower, "filename") {
			continue
		}
		_, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"`)
		if strings.HasPrefix(strings.ToLower(part), "filename*") {
			// 形如 UTF-8''%E8%AE%BA%E6%96%87.pdf
			if _, encoded, found := strings.Cut(value, "''"); found {
				decoded, err := url.QueryUnescape(encoded)
				if err == nil {
					value = decoded
				}
			}
		}
		if strings.TrimSpace(value) != "" {
			return sanitizeServerFileName(value)
		}
	}
	return ""
}

// sanitizeServerFileName 只取服务端文件名的最后一段, 防止它带着路径.
func sanitizeServerFileName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	if index := strings.LastIndex(name, "/"); index >= 0 {
		name = name[index+1:]
	}
	return strings.TrimSpace(name)
}

// previewBytes 把字节串渲染成可读预览, 用于诊断.
func previewBytes(data []byte, limit int) string {
	if len(data) == 0 {
		return "(空)"
	}
	if len(data) > limit {
		data = data[:limit]
	}
	return fmt.Sprintf("%q", data)
}
