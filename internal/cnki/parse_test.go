package cnki

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// 下列 fixture 按知网真实响应结构裁剪而成, 只保留解析器关心的部分.
// 保留真实结构而不是自造简化标签, 才能让测试真正守住解析逻辑.

const resultTableFixture = `
<div id="briefBox">
  <div class="result-con-r" id="countPageDiv">
    <span class="pagerTitleCell"><span>共找到</span> <em>34,967</em> <span>条结果</span></span>
    <span class="countPageMark" data-pagenum="300">1/300</span>
  </div>
  <table class="result-table-list">
    <thead><tr>
      <th></th><th>题名</th><th>作者</th><th>来源</th><th>发表时间</th>
      <th>数据库</th><th>被引</th><th>下载</th><th>操作</th>
    </tr></thead>
    <tbody>
      <tr>
        <td class="seq"><input class="cbItem" type="checkbox" data-cur="1" name="CookieName"
            value="FAKE0token0for0tests0AAAA_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb!"
            onclick="$(this).filenameClick()">1</td>
        <td class="name"><a class="fz14 inline" target="_blank"
            href="https://kns.cnki.net/kcms2/article/abstract?v=FAKEdetailToken0aaa">RAG-Maldetect：基于检索增强生成的<font color="red">安卓</font>恶意软件检测方法</a><b class="marktip">网络首发</b><i class="freeIdent is-hide" title="Free" style="display: none;">免费</i></td>
        <td class="author"><a class="KnowledgeNetLink" href="https://kns.cnki.net/kcms2/author/detail?v=aaa">何易临</a>;<a class="KnowledgeNetLink" href="https://kns.cnki.net/kcms2/author/detail?v=bbb">丁锰</a>;<a class="KnowledgeNetLink" href="https://kns.cnki.net/kcms2/author/detail?v=ccc">许晓宇</a></td>
        <td class="source"><p><a target="_blank" href="https://navi.cnki.net/knavi/detail?p=zzz">计算机工程与应用</a></p></td>
        <td class="date">2024-03-15</td>
        <td class="data">期刊</td>
        <td class="cited"><a href="javascript:void(0)">12</a></td>
        <td class="download"><a href="javascript:void(0)">1,024</a></td>
        <td class="operat"><a href="javascript:void(0)">下载</a></td>
      </tr>
      <tr>
        <td class="seq"><input class="cbItem" type="checkbox" value="secondToken!">2</td>
        <td class="name"><a class="fz14 inline" target="_blank"
            href="/kcms2/article/abstract?v=relativeToken">深度学习在医学影像中的应用</a></td>
        <td class="author"><a class="KnowledgeNetLink" href="#">张三</a></td>
        <td class="source">中国医学影像学杂志</td>
        <td class="date">2023-01-02</td>
        <td class="data">期刊</td>
        <td class="cited"><a href="javascript:void(0)">0</a></td>
        <td class="download"><a href="javascript:void(0)">0</a></td>
        <td class="operat"></td>
      </tr>
    </tbody>
  </table>
</div>`

func TestParseSearchResult(t *testing.T) {
	result, err := ParseSearchResult(resultTableFixture, 1, 20)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if result.Total != 34967 {
		t.Errorf("总命中数解析错误, 期望 34967, 实际 %d", result.Total)
	}
	if result.PageCount != 300 {
		t.Errorf("总页数解析错误, 期望 300, 实际 %d", result.PageCount)
	}
	if len(result.Articles) != 2 {
		t.Fatalf("应解析出 2 条, 实际 %d 条", len(result.Articles))
	}

	first := result.Articles[0]
	// 标题里的高亮 font 标签不应污染文本.
	if first.Title != "RAG-Maldetect：基于检索增强生成的安卓恶意软件检测方法" {
		t.Errorf("标题解析错误: %q", first.Title)
	}
	// 作者以分号分隔, 且不应带上单位角标.
	if len(first.Authors) != 3 || first.Authors[1] != "丁锰" {
		t.Errorf("作者解析错误: %v", first.Authors)
	}
	if first.Source != "计算机工程与应用" {
		t.Errorf("来源解析错误: %q", first.Source)
	}
	if first.Date != "2024-03-15" {
		t.Errorf("日期解析错误: %q", first.Date)
	}
	if first.Dbname != "期刊" {
		t.Errorf("数据库类型解析错误: %q", first.Dbname)
	}
	// 被引与下载带千分位, 应被正确去掉.
	if first.CitedCount != 12 {
		t.Errorf("被引数解析错误: %d", first.CitedCount)
	}
	if first.DownCount != 1024 {
		t.Errorf("下载数解析错误: %d", first.DownCount)
	}
	// 复选框的值是导出所需的加密标识.
	if first.Filename == "" {
		t.Error("未取到加密文献标识")
	}
	if first.Type != "网络首发" {
		t.Errorf("形态标记解析错误: %q", first.Type)
	}
	if first.HasFullTxt {
		t.Error("免费标识处于隐藏状态, 不应判定为有全文")
	}

	second := result.Articles[1]
	if second.Index != 2 {
		t.Errorf("序号应从 1 连续编号, 实际 %d", second.Index)
	}
	// 相对地址应被补全为绝对地址.
	if second.DetailURL != "https://kns.cnki.net/kcms2/article/abstract?v=relativeToken" {
		t.Errorf("相对地址未补全: %q", second.DetailURL)
	}
	// 无被引无下载时应为 0 而不是解析失败残留值.
	if second.CitedCount != 0 || second.DownCount != 0 {
		t.Errorf("零值解析错误: 被引 %d 下载 %d", second.CitedCount, second.DownCount)
	}
}

func TestParseSearchResultEmpty(t *testing.T) {
	// 空结果时知网只返回一句提示, 这属于正常情况.
	empty := `<div id="briefBox"><p class="no-content" value="">抱歉，暂无数据，请稍后重试。</p></div>`
	result, err := ParseSearchResult(empty, 1, 20)
	if err != nil {
		t.Fatalf("空结果不应报错: %v", err)
	}
	if len(result.Articles) != 0 {
		t.Errorf("空结果不应有条目, 实际 %d 条", len(result.Articles))
	}
}

func TestParseSearchResultMalformed(t *testing.T) {
	// 结构完全对不上时应当报错, 而不是静默返回空列表.
	if _, err := ParseSearchResult(`<html><body><div>完全不是结果页</div></body></html>`, 1, 20); err == nil {
		t.Error("结构不符时应报错")
	}
}

const sentenceFixture = `
<div id="briefBox">
  <div id="countPageDiv"><span class="pagerTitleCell"><span>共找到</span> <em>79</em> <span>条结果</span></span>
  <span class="countPageMark">1/4</span></div>
  <div class="middle">
    <h5><label>句子1：</label><font color="red">大语言模型</font>在数学推理、代码生成、<font color="red">医学影像</font>诊断等领域展现出能力。<br></h5>
    <h6><label>句子来自：</label><span><a class="fz14" target="_blank"
        href="https://kns.cnki.net/kcms2/article/abstract?v=senToken">人工智能时代人才能力的重构与培育研究</a></span></h6>
    <div class="baseinfo">
      <span><label>作者：</label><a class="KnowledgeNetLink" href="#">何彦杰;</a><a class="KnowledgeNetLink" href="#">倪凯</a></span>
      <em>【期刊】</em>
      <span><label>来源：</label><a target="_blank" href="https://navi.cnki.net/knavi/detail?p=x">科技创业月刊</a></span>
      <span class="date">2026-09-21</span>
      <span class="opts-count">下载<a class="downloadCnt" href="javascript:void(0);"><em>32</em></a></span>
    </div>
  </div>
</div>`

func TestParseSentenceResult(t *testing.T) {
	result, err := ParseSentenceResult(sentenceFixture, 1, 20)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if result.Total != 79 {
		t.Errorf("总命中数解析错误: %d", result.Total)
	}
	if len(result.Articles) != 1 {
		t.Fatalf("应解析出 1 条, 实际 %d 条", len(result.Articles))
	}

	article := result.Articles[0]
	if article.Sentence == "" {
		t.Fatal("未取到命中原句")
	}
	// 原句前的 "句子1：" 标签应被清理掉.
	if strings.HasPrefix(article.Sentence, "句子") {
		t.Errorf("原句前缀未被清理: %q", article.Sentence)
	}
	if article.Title != "人工智能时代人才能力的重构与培育研究" {
		t.Errorf("出处标题解析错误: %q", article.Title)
	}
	if len(article.Authors) != 2 || article.Authors[0] != "何彦杰" {
		t.Errorf("作者解析错误: %v (分号应被去掉)", article.Authors)
	}
	if article.Source != "科技创业月刊" {
		t.Errorf("来源解析错误: %q", article.Source)
	}
	if article.Type != "期刊" {
		t.Errorf("文献类型解析错误: %q", article.Type)
	}
	if article.Date != "2026-09-21" {
		t.Errorf("日期解析错误: %q", article.Date)
	}
	if article.DownCount != 32 {
		t.Errorf("下载数解析错误: %d", article.DownCount)
	}
}

func TestParseGroups(t *testing.T) {
	fixture := `<div id="divGroup">
  <dl groupid="ZYZT|||CYZT">
    <dt class="tit" groupid="ZYZT|||CYZT"><b>主题</b></dt>
    <dd field="ZYZT" tit="主要主题">
      <ul><li><input type="checkbox" value="Android" text="Android"> <a>Android</a> <span>(1.41万)</span></li></ul>
    </dd>
  </dl>
  <dl groupid="CCL">
    <dt class="tit"><b>学科</b></dt>
    <dd field="CCL" tit="学科">
      <ul><li><input type="checkbox" value="I140" text="自动化技术"> <a>自动化技术</a> <span>(8464)</span></li></ul>
    </dd>
  </dl>
</div>`

	groups, err := ParseGroups(fixture)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("应解析出 2 个分组, 实际 %d 个", len(groups))
	}
	if groups[0].Label != "主题" {
		t.Errorf("分组标题解析错误: %q", groups[0].Label)
	}
	// 学科分组的展示文本在 text 属性上, value 只是内部代码.
	if groups[1].Items[0].Value != "自动化技术" {
		t.Errorf("应优先取 text 属性作为展示值, 实际 %q", groups[1].Items[0].Value)
	}
	if groups[1].Items[0].Count != 8464 {
		t.Errorf("分组计数解析错误: %d", groups[1].Items[0].Count)
	}
}

func TestParseDetail(t *testing.T) {
	fixture := `<html><head><title>某论文 - 中国知网</title></head><body>
  <h1>基于深度学习的图像识别 附视频</h1>
  <h3 class="author">张三1 李四2</h3>
  <h3 class="author"><a href="#">王五</a><a href="#">1.某某大学计算机学院</a></h3>
  <div id="ChDivSummary">这是一段摘要文本。</div>
  <p class="keywords"><a>深度学习</a><a>图像识别</a></p>
  <p>分类号：TP391.41</p>
  <p class="funds">基金：国家自然科学基金（12345678）</p>
</body></html>`

	article, err := ParseDetail(fixture)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	// 标题末尾的形态后缀应被去掉.
	if article.Title != "基于深度学习的图像识别" {
		t.Errorf("标题未清理后缀: %q", article.Title)
	}
	if article.Abstract != "这是一段摘要文本。" {
		t.Errorf("摘要解析错误: %q", article.Abstract)
	}
	if len(article.Keywords) != 2 {
		t.Errorf("关键词解析错误: %v", article.Keywords)
	}
	if article.CLC != "TP391.41" {
		t.Errorf("分类号解析错误: %q", article.CLC)
	}
	if article.Fund == "" {
		t.Error("基金解析失败")
	}
	// 机构不应混进作者列表.
	for _, author := range article.Authors {
		if author == "1.某某大学计算机学院" {
			t.Errorf("机构被误判为作者: %v", article.Authors)
		}
	}
}

func TestParseDetailMissingTitle(t *testing.T) {
	if _, err := ParseDetail(`<html><body><div>登录</div></body></html>`); err == nil {
		t.Error("无标题时应报错")
	}
}

func TestDetailPathNormalization(t *testing.T) {
	cases := map[string]string{
		"https://kns.cnki.net/kcms2/article/abstract?v=abc": "/kcms2/article/abstract?v=abc",
		"/kcms2/article/abstract?v=abc":                     "/kcms2/article/abstract?v=abc",
		"abc":                                               "/kcms2/article/abstract?v=abc",
	}
	for input, want := range cases {
		if got := DetailPath(input); got != want {
			t.Errorf("地址规整错误: 输入 %q, 期望 %q, 实际 %q", input, want, got)
		}
	}
}

// TestParseSearchResultReportsRejection 校验知网拒绝请求时不再静默返回空结果.
//
// 知网用同一个 p.no-content 同时表达"无结果"与"拒绝请求", 区别在于拒绝时
// 它把原因写在 value 属性里. 若不区分, 用户会把检索式写错误认为是没有匹配.
func TestParseSearchResultReportsRejection(t *testing.T) {
	rejected := `<div id="briefBox"><p class="no-content" value="查询对象结构错误, 没有指定检索分类！">抱歉，暂无数据，请稍后重试。</p></div>`

	_, err := ParseSearchResult(rejected, 1, 20)
	if err == nil {
		t.Fatal("知网拒绝请求时应报错, 而不是当作空结果返回")
	}
	// 错误信息必须带上知网的原话, 否则用户无从判断哪里写错了.
	if !strings.Contains(err.Error(), "没有指定检索分类") {
		t.Errorf("错误信息应包含知网给出的原因, 实际: %v", err)
	}
}

// TestParseSearchResultMalformedCarriesContext 校验结构无法识别时的错误信息带够上下文.
//
// 只有一句"知网可能已改版"无法区分改版, 被拦截与需要登录, 因此要求把
// 响应体的特征一并带出.
func TestParseSearchResultMalformedCarriesContext(t *testing.T) {
	page := `<html><head><title>访问受限</title></head><body><p>请求过于频繁</p></body></html>`

	_, err := ParseSearchResult(page, 1, 20)
	if err == nil {
		t.Fatal("结构无法识别时应报错")
	}

	message := err.Error()
	for _, want := range []string{"访问受限", "字节", "请求过于频繁"} {
		if !strings.Contains(message, want) {
			t.Errorf("错误信息缺少 %q, 实际: %s", want, message)
		}
	}
}

// TestFirstHintValueSkipsFormControls 校验诊断信息不会取到表单控件的取值.
func TestFirstHintValueSkipsFormControls(t *testing.T) {
	// 表单控件的 value 是正常数据, 不能当成知网的提示.
	page := `<html><body>
		<input type="hidden" id="classid" value="WD0FTY92">
		<p value="真正的提示">正文</p>
	</body></html>`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(page))
	if err != nil {
		t.Fatalf("构造文档失败: %v", err)
	}
	if got := firstHintValue(doc); got != "真正的提示" {
		t.Errorf("应跳过表单控件取到真正的提示, 实际 %q", got)
	}
}
