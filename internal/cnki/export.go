package cnki

import (
	"context"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/azazo1/cnki-cli/internal/apperr"
)

// DocumentManageHost 是知网文献管理服务地址, 题录导出与部分后处理都走这里.
//
// 取值来自结果页隐藏域 #hidDocumentManageUrl 的实测值.
const DocumentManageHost = "https://kns.cnki.net/dm8"

// PathExport 是题录导出接口地址.
const PathExport = "/manage/export.html"

// ExportRequest 描述一次题录导出请求.
type ExportRequest struct {
	// DisplayMode 是知网导出格式标识, 见 taxonomy.ExportFormats.
	DisplayMode string
	// Filenames 是结果页给出的加密文献标识.
	//
	// 该标识只能在检索结果页拿到, 无法自行构造, 因此导出总是跟在一次
	// 检索之后. 多条以 "!" 分隔的原始 value 直接拼接即可.
	Filenames []string
	// SearchInfo 是导出页展示的检索来源信息, 形如 "1!总库!>主题：android".
	SearchInfo string
	// Subject 是可选的专题标识, 普通检索留空.
	Subject string
	// MapIndex 是知网导出页要求的位置参数, 普通用法固定为 null.
	MapIndex string
}

// Export 发起题录导出请求, 返回导出服务生成的题录正文.
//
// 知网该接口先返回 302, 真正的题录内容在 Location 指向的地址上, 因此这里
// 需要额外跟随一次重定向. 客户端自身配置的是不自动跟随重定向 (以便识别
// 验证跳转), 所以这一步显式完成.
func (c *Client) Export(ctx context.Context, req ExportRequest) (string, error) {
	if len(req.Filenames) == 0 {
		return "", apperr.Usage("没有要导出的文献, 请先执行检索")
	}
	if req.DisplayMode == "" {
		return "", apperr.Usage("未指定导出格式")
	}

	form := url.Values{}
	form.Set("displaymode", req.DisplayMode)
	form.Set("filename", strings.Join(req.Filenames, ""))
	form.Set("searchinfo", req.SearchInfo)
	form.Set("mapIndex", orNull(req.MapIndex))
	if req.Subject != "" {
		form.Set("subject", req.Subject)
	}

	resp, err := c.PostForm(ctx, DocumentManageHost+PathExport, form)
	if err != nil {
		return "", err
	}

	body := resp.Text()
	// 导出服务把结果放在重定向目标上, 这里跟进一次.
	location := strings.TrimSpace(resp.Header.Get("Location"))
	if location != "" {
		body = c.followExportRedirect(ctx, location, req)
	}

	if isLoginPage(body) || isLoginURL(location) {
		return "", apperr.Permission(
			"该导出格式由知网服务生成, 需要登录知网个人账号. " +
				"当前会话只有机构 IP 权限, 可改用本地生成的 ris 或 bibtex 格式, " +
				"或用 cnki auth login 在浏览器窗口内登录个人账号后重试")
	}
	if text, err := ExtractExportBody(body); err == nil && strings.TrimSpace(text) != "" {
		return text, nil
	}
	if strings.TrimSpace(body) == "" {
		return "", apperr.Remote("知网导出服务未返回任何内容")
	}
	return strings.TrimSpace(body), nil
}

// isLoginURL 判断地址是否指向知网登录服务.
func isLoginURL(location string) bool {
	return strings.Contains(location, "login.cnki.net") || strings.Contains(location, "/login")
}

// isLoginPage 判断响应体是否是知网登录页.
//
// 知网导出在缺少个人账号权限时会 302 到 login.cnki.net, 若不加识别就会把
// 整页登录 HTML 当作题录输出, 用户拿到的将是一堆无意义标记.
func isLoginPage(body string) bool {
	if len(body) > 200_000 {
		// 真正的题录不会这么大, 避免在全量结果页上做无谓匹配.
		return false
	}
	head := body
	if len(head) > 4096 {
		head = head[:4096]
	}
	return strings.Contains(head, "ecpLogin") ||
		strings.Contains(head, "TopLoginCore") ||
		strings.Contains(head, "中国知网-登录")
}

// followExportRedirect 跟进导出服务的重定向.
//
// 知网在不同格式下会用 GET 或 POST 承载参数, 因此先按 GET 取, 明确失败后
// 再用同样的表单以 POST 重试. 都失败时返回空串, 由调用方退回到原始响应.
func (c *Client) followExportRedirect(ctx context.Context, location string, req ExportRequest) string {
	target := c.AbsURL(location)
	if loc, err := url.Parse(location); err == nil && loc.IsAbs() {
		target = location
	}

	if resp, err := c.Get(ctx, target, nil); err == nil {
		if text := resp.Text(); len(text) > 0 {
			return text
		}
	}

	form := url.Values{}
	form.Set("displaymode", req.DisplayMode)
	form.Set("filename", strings.Join(req.Filenames, ""))
	form.Set("searchinfo", req.SearchInfo)
	form.Set("mapIndex", orNull(req.MapIndex))
	if resp, err := c.PostForm(ctx, target, form); err == nil {
		return resp.Text()
	}
	return ""
}

// orNull 把空值渲染为知网要求的字面量 null.
func orNull(value string) string {
	if strings.TrimSpace(value) == "" {
		return "null"
	}
	return value
}

// BuildSearchInfo 构造导出页需要的检索来源描述.
//
// 观测到的真实形态是 "1!总库!>主题：android", 其中首段是检索来源编号,
// 第二段是库名, 末段是检索式描述.
func BuildSearchInfo(dbName string, condition string) string {
	if strings.TrimSpace(dbName) == "" {
		dbName = "总库"
	}
	condition = strings.TrimSpace(condition)
	return "1!" + dbName + "!>" + condition
}

// ExtractExportBody 从知网导出结果页中抽取题录正文.
//
// 导出服务返回的是整张 HTML 页面, 真正的题录内容在 textarea, pre 或
// 结果区块中. 这里按可能性从高到低尝试, 都拿不到时返回空串由调用方决定
// 如何降级.
func ExtractExportBody(html string) (string, error) {
	if strings.TrimSpace(html) == "" {
		return "", nil
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return "", err
	}

	selectors := []string{
		"textarea#result",
		"textarea#exportContent",
		"textarea",
		"pre#content",
		"pre",
		"#content",
		".export-content",
		".main-content",
	}
	for _, selector := range selectors {
		candidate := doc.Find(selector).First()
		if candidate.Length() == 0 {
			continue
		}
		text := candidate.Text()
		if selector == "textarea" || strings.HasPrefix(selector, "textarea") {
			// textarea 的正文在 value 属性里, Text() 取不到.
			if value, ok := candidate.Attr("value"); ok && strings.TrimSpace(value) != "" {
				text = value
			}
		}
		text = strings.TrimSpace(text)
		if text != "" {
			return text, nil
		}
	}
	return "", nil
}
