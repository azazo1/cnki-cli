package cnki

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

// 下列 fixture 按知网真实详情页的结构裁剪, 覆盖两种文案风格:
// 期刊写 "PDF下载"/"CAJ下载", 专利写 "CAJ原文下载".
const journalDetailFixture = `<html><body>
  <h1>某篇期刊论文</h1>
  <ul class="btn-opt">
    <li class="btn-yb-reader"><a href="https://bar.cnki.net/bar/download/order?id=read-journal">原版阅读</a></li>
    <li class="btn-dlcaj"><a href="https://bar.cnki.net/bar/download/order?id=caj-journal">CAJ下载</a></li>
    <li class="btn-dlpdf"><a href="https://bar.cnki.net/bar/download/order?id=pdf-journal">PDF下载</a></li>
  </ul>
</body></html>`

const patentDetailFixture = `<html><body>
  <h1>某件专利</h1>
  <ul class="btn-opt">
    <li class="btn-qwxz"><a class="a05" href="https://bar.cnki.net/bar/download/order?id=caj-patent">CAJ原文下载</a></li>
  </ul>
</body></html>`

func TestParseDownloadOrderURLJournal(t *testing.T) {
	pdf, err := ParseDownloadOrderURL(journalDetailFixture, FormatPDF)
	if err != nil {
		t.Fatalf("取 PDF 入口失败: %v", err)
	}
	if !strings.HasSuffix(pdf, "id=pdf-journal") {
		t.Errorf("PDF 入口取错: %q", pdf)
	}

	caj, err := ParseDownloadOrderURL(journalDetailFixture, FormatCAJ)
	if err != nil {
		t.Fatalf("取 CAJ 入口失败: %v", err)
	}
	if !strings.HasSuffix(caj, "id=caj-journal") {
		t.Errorf("CAJ 入口取错: %q", caj)
	}
}

// TestParseDownloadOrderURLPatentLabel 校验专利的 "CAJ原文下载" 也能被认出.
//
// 实测专利详情页用的就是这个词, 早期按完整文案 "CAJ下载" 匹配会漏掉它.
func TestParseDownloadOrderURLPatentLabel(t *testing.T) {
	caj, err := ParseDownloadOrderURL(patentDetailFixture, FormatCAJ)
	if err != nil {
		t.Fatalf("专利的 CAJ 入口应当被认出: %v", err)
	}
	if !strings.HasSuffix(caj, "id=caj-patent") {
		t.Errorf("入口取错: %q", caj)
	}

	// 专利没有提供 PDF, 应当明确报出缺少该格式, 而不是返回空地址.
	if _, err := ParseDownloadOrderURL(patentDetailFixture, FormatPDF); err == nil {
		t.Error("没有 PDF 入口时应当报错")
	}
}

func TestParseDownloadOrderURLMatchesParentClass(t *testing.T) {
	// 文案被改写成不含关键字时, 仍应能靠父元素类名认出入口.
	fixture := `<html><body>
	  <ul><li class="btn-dlpdf"><a href="https://bar.cnki.net/bar/download/order?id=byclass">全文</a></li></ul>
	</body></html>`
	got, err := ParseDownloadOrderURL(fixture, FormatPDF)
	if err != nil {
		t.Fatalf("应靠父类名认出入口: %v", err)
	}
	if !strings.HasSuffix(got, "id=byclass") {
		t.Errorf("入口取错: %q", got)
	}
}

func TestParseDownloadFormat(t *testing.T) {
	cases := map[string]DownloadFormat{
		"pdf":  FormatPDF,
		"PDF":  FormatPDF,
		"":     FormatPDF,
		" caj": FormatCAJ,
		"Caj":  FormatCAJ,
	}
	for input, want := range cases {
		got, err := ParseDownloadFormat(input)
		if err != nil {
			t.Errorf("解析 %q 失败: %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("解析 %q 得到 %q, 期望 %q", input, got, want)
		}
	}
	if _, err := ParseDownloadFormat("docx"); err == nil {
		t.Error("未知格式应当报错")
	}
}

func TestClassifyDownloadByMagic(t *testing.T) {
	// 文件头是硬证据: 即使服务端没给出文件名, 也应能认出来.
	pdf := &Response{StatusCode: 200, Header: http.Header{}, Body: []byte("%PDF-1.7\n...")}
	result, err := classifyDownload(pdf)
	if err != nil {
		t.Fatalf("识别 PDF 失败: %v", err)
	}
	if result.Format != FormatPDF {
		t.Errorf("格式识别错误: %q", result.Format)
	}
	if result.FileName == "" {
		t.Error("应给出兜底文件名")
	}

	caj := &Response{StatusCode: 200, Header: http.Header{}, Body: []byte("CAJ\x00rest")}
	result, err = classifyDownload(caj)
	if err != nil {
		t.Fatalf("识别 CAJ 失败: %v", err)
	}
	if result.Format != FormatCAJ {
		t.Errorf("格式识别错误: %q", result.Format)
	}
}

func TestClassifyDownloadByDisposition(t *testing.T) {
	header := http.Header{}
	header.Set("Content-Disposition", `attachment; filename="paper.pdf"`)
	resp := &Response{StatusCode: 200, Header: header, Body: []byte("不认识的字节")}

	result, err := classifyDownload(resp)
	if err != nil {
		t.Fatalf("应能靠文件名认出来: %v", err)
	}
	if result.Format != FormatPDF {
		t.Errorf("格式识别错误: %q", result.Format)
	}
	if result.FileName != "paper.pdf" {
		t.Errorf("文件名解析错误: %q", result.FileName)
	}
}

func TestClassifyDownloadRejectsUnrecognized(t *testing.T) {
	resp := &Response{
		StatusCode: 200,
		Header:     http.Header{},
		Body:       []byte("<html><body>这是一个说明页</body></html>"),
	}
	_, err := classifyDownload(resp)
	if err == nil {
		t.Fatal("无法识别的响应应当报错, 而不是当成文件存下来")
	}
	// 报错要带上足够定位问题的信息.
	for _, want := range []string{"字节", "开头字节"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("错误信息缺少 %q: %v", want, err)
		}
	}
}

func TestClassifyDownloadDetectsLoginPage(t *testing.T) {
	// 没有权限时知网也可能直接返回登录页, 这要报成权限问题而不是格式未知.
	login := `<html><head><script src="css/ecpLogin.css"></script></head><body>中国知网-登录</body></html>`
	resp := &Response{StatusCode: 200, Header: http.Header{}, Body: []byte(login)}

	_, err := classifyDownload(resp)
	if !errors.Is(err, ErrLoginRequired) {
		t.Errorf("应当识别为需要登录, 实际: %v", err)
	}
}

func TestFilenameFromDisposition(t *testing.T) {
	cases := map[string]string{
		`attachment; filename="paper.pdf"`:                    "paper.pdf",
		`attachment;filename=plain.pdf`:                       "plain.pdf",
		`attachment; filename*=UTF-8''%E8%AE%BA%E6%96%87.pdf`: "论文.pdf",
		`inline`: "",
		``:       "",
		// 服务端给出路径时只取最后一段, 避免写到目录之外.
		`attachment; filename="/etc/passwd"`: "passwd",
	}
	for input, want := range cases {
		if got := filenameFromDisposition(input); got != want {
			t.Errorf("解析 %q 得到 %q, 期望 %q", input, got, want)
		}
	}
}

func TestResolveRedirect(t *testing.T) {
	// 知网用协议相对地址, 必须按 net/url 的规则解析, 不能手工拼接.
	cases := []struct{ current, location, want string }{
		{
			"https://bar.cnki.net/bar/download/order?id=x",
			"//login.cnki.net/login/?returnUrl=y",
			"https://login.cnki.net/login/?returnUrl=y",
		},
		{
			"https://bar.cnki.net/bar/download/order?id=x",
			"/bar/download/file.pdf",
			"https://bar.cnki.net/bar/download/file.pdf",
		},
		{
			"https://bar.cnki.net/bar/download/order?id=x",
			"https://cdn.cnki.net/a.pdf",
			"https://cdn.cnki.net/a.pdf",
		},
	}
	for _, c := range cases {
		got, err := resolveRedirect(c.current, c.location)
		if err != nil {
			t.Errorf("解析失败: %v", err)
			continue
		}
		if got != c.want {
			t.Errorf("由 %q 跳到 %q 得到 %q, 期望 %q", c.current, c.location, got, c.want)
		}
	}
}

func TestIsLoginRequired(t *testing.T) {
	if !IsLoginRequired(ErrLoginRequired) {
		t.Error("应识别自身错误")
	}
	if IsLoginRequired(errors.New("别的错误")) {
		t.Error("不应把别的错误当成权限问题")
	}
	if IsLoginRequired(nil) {
		t.Error("nil 不应被当成权限问题")
	}
}
