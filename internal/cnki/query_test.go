package cnki

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/azazo1/cnki-cli/internal/taxonomy"
)

// TestSimpleQueryJSONShape 校验一框式检索的 QueryJson 形态.
//
// 该结构直接对应知网 brief 接口的入参, 一旦改动就会导致检索失效,
// 因此逐字段核对.
func TestSimpleQueryJSONShape(t *testing.T) {
	q, err := SimpleQuery("android", "SU", SearchOptions{})
	if err != nil {
		t.Fatalf("构造检索式失败: %v", err)
	}

	raw, err := q.MarshalQueryJSON()
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("解析自身序列化结果失败: %v", err)
	}

	if decoded["Resource"] != "CROSSDB" {
		t.Errorf("Resource 应为 CROSSDB, 实际 %v", decoded["Resource"])
	}
	if decoded["Classid"] != taxonomy.TotalLibraryClassid {
		t.Errorf("Classid 应为 %s, 实际 %v", taxonomy.TotalLibraryClassid, decoded["Classid"])
	}
	if decoded["SearchType"] != float64(taxonomy.SearchSimple) {
		t.Errorf("一框式检索的 SearchType 应为 %d, 实际 %v", taxonomy.SearchSimple, decoded["SearchType"])
	}
	if decoded["KuaKuCode"] != taxonomy.TotalLibraryKuaKuCode {
		t.Errorf("KuaKuCode 与总库默认值不一致")
	}

	node, ok := decoded["QNode"].(map[string]any)
	if !ok {
		t.Fatalf("QNode 结构异常")
	}
	groups, ok := node["QGroup"].([]any)
	if !ok || len(groups) != 1 {
		t.Fatalf("一框式检索应只有一组检索项, 实际 %v", node["QGroup"])
	}
	group := groups[0].(map[string]any)
	if group["Key"] != GroupSubject {
		t.Errorf("组名应为 %s, 实际 %v", GroupSubject, group["Key"])
	}
	items := group["Items"].([]any)
	if len(items) != 1 {
		t.Fatalf("应只有一个检索项, 实际 %d 个", len(items))
	}
	item := items[0].(map[string]any)
	if item["Field"] != "SU" {
		t.Errorf("字段应为 SU, 实际 %v", item["Field"])
	}
	if item["Value"] != "android" {
		t.Errorf("检索词应为 android, 实际 %v", item["Value"])
	}
	// 主题字段使用相关度优先的匹配方式.
	if item["Operator"] != taxonomy.MatchTopRank {
		t.Errorf("主题检索的 Operator 应为 %s, 实际 %v", taxonomy.MatchTopRank, item["Operator"])
	}
}

// TestExpertQueryJSONShape 校验专业检索的 QueryJson 形态.
func TestExpertQueryJSONShape(t *testing.T) {
	q, err := ExpertQuery("SU=('android')", SearchOptions{})
	if err != nil {
		t.Fatalf("构造检索式失败: %v", err)
	}
	payload := q.QueryJSON()

	if payload.SearchType != int(taxonomy.SearchExpert) {
		t.Errorf("专业检索的 SearchType 应为 %d", taxonomy.SearchExpert)
	}
	// 专业检索需要额外的控制条件组.
	if len(payload.QNode.QGroup) != 2 {
		t.Fatalf("专业检索应有两组, 实际 %d 组", len(payload.QNode.QGroup))
	}
	if payload.QNode.QGroup[1].Key != GroupControl {
		t.Errorf("第二组应为 %s, 实际 %s", GroupControl, payload.QNode.QGroup[1].Key)
	}
	item := payload.QNode.QGroup[0].Items[0]
	if item.Field != FieldExpert {
		t.Errorf("专业检索字段应为 %s, 实际 %s", FieldExpert, item.Field)
	}
	if item.Value != "SU=('android')" {
		t.Errorf("表达式未被正确传递: %s", item.Value)
	}
	// 专业检索的 Operator 是数字形态.
	if _, ok := item.Operator.(int); !ok {
		t.Errorf("专业检索的 Operator 应为数字, 实际 %T", item.Operator)
	}
}

// TestSentenceQueryUsesFullTextField 校验句子检索落在全文字段上.
//
// 实测若改用主题字段或拆成两个检索项, 知网会返回
// "查询对象结构错误, 没有指定检索分类".
func TestSentenceQueryUsesFullTextField(t *testing.T) {
	q, err := SentenceQuery("大语言模型", "医学影像", false, SearchOptions{})
	if err != nil {
		t.Fatalf("构造句子检索失败: %v", err)
	}
	payload := q.QueryJSON()
	if payload.SearchType != int(taxonomy.SearchSentence) {
		t.Errorf("SearchType 应为 %d, 实际 %d", taxonomy.SearchSentence, payload.SearchType)
	}
	items := payload.QNode.QGroup[0].Items
	if len(items) != 1 {
		t.Fatalf("句子检索应只有一个检索项, 实际 %d 个", len(items))
	}
	if items[0].Field != SentenceField {
		t.Errorf("句子检索字段应为 %s, 实际 %s", SentenceField, items[0].Field)
	}
	if items[0].Value2 != "医学影像" {
		t.Errorf("第二个检索词应放在 Value2, 实际 %q", items[0].Value2)
	}
	if items[0].Operator != int(taxonomy.OpNear) {
		t.Errorf("同一句应使用 OpNear, 实际 %v", items[0].Operator)
	}

	para, err := SentenceQuery("大语言模型", "医学影像", true, SearchOptions{})
	if err != nil {
		t.Fatalf("构造同一段句子检索失败: %v", err)
	}
	if got := para.QueryJSON().QNode.QGroup[0].Items[0].Operator; got != int(taxonomy.OpSentence) {
		t.Errorf("同一段应使用 OpSentence, 实际 %v", got)
	}
}

// TestFormContainsRequiredFields 校验表单必填参数齐全.
//
// searchFrom 缺失时知网会返回 "查询对象结构错误", 因此它是硬性要求.
func TestFormContainsRequiredFields(t *testing.T) {
	q, err := SimpleQuery("android", "", SearchOptions{Page: 2, PageSize: 50})
	if err != nil {
		t.Fatalf("构造检索式失败: %v", err)
	}
	form, err := q.Form()
	if err != nil {
		t.Fatalf("构造表单失败: %v", err)
	}

	required := []string{"boolSearch", "QueryJson", "pageNum", "pageSize",
		"sortField", "sortType", "dstyle", "productStr", "aside",
		"searchFrom", "subject", "language", "uniplatform", "CurPage"}
	for _, key := range required {
		if _, ok := form[key]; !ok {
			t.Errorf("表单缺少必填参数 %s", key)
		}
	}
	if form.Get("pageNum") != "2" {
		t.Errorf("页码未生效, 实际 %s", form.Get("pageNum"))
	}
	if form.Get("pageSize") != "50" {
		t.Errorf("每页条数未生效, 实际 %s", form.Get("pageSize"))
	}
	if strings.TrimSpace(form.Get("searchFrom")) == "" {
		t.Error("searchFrom 不能为空")
	}
	// QueryJson 必须是可解析的 JSON.
	if _, err := url.QueryUnescape(form.Get("QueryJson")); err != nil {
		t.Errorf("QueryJson 编码异常: %v", err)
	}
}

// TestSentenceFormCarriesFlag 校验句子检索带上 sentenceSearch 标志.
func TestSentenceFormCarriesFlag(t *testing.T) {
	q, err := SentenceQuery("词一", "词二", false, SearchOptions{})
	if err != nil {
		t.Fatalf("构造失败: %v", err)
	}
	form, err := q.Form()
	if err != nil {
		t.Fatalf("构造表单失败: %v", err)
	}
	if form.Get("sentenceSearch") != "true" {
		t.Error("句子检索必须带 sentenceSearch=true")
	}
}

// TestSimpleQueryRejectsUnknownField 校验未知字段被拒绝.
func TestSimpleQueryRejectsUnknownField(t *testing.T) {
	if _, err := SimpleQuery("x", "NOT_A_FIELD", SearchOptions{}); err == nil {
		t.Error("未知检索字段应当报错")
	}
	if _, err := SimpleQuery("   ", "", SearchOptions{}); err == nil {
		t.Error("空检索词应当报错")
	}
}

// TestAdvancedQueryRejectsEmpty 校验高级检索的参数校验.
func TestAdvancedQueryRejectsEmpty(t *testing.T) {
	if _, err := AdvancedQuery(nil, SearchOptions{}); err == nil {
		t.Error("无条件时应报错")
	}
	if _, err := AdvancedQuery([]Condition{{Field: "SU", Value: " "}}, SearchOptions{}); err == nil {
		t.Error("检索词为空时应报错")
	}
}

// TestOptionsDefaults 校验控制参数缺省值.
func TestOptionsDefaults(t *testing.T) {
	opts := SearchOptions{}.withDefaults()
	if opts.Classid != taxonomy.TotalLibraryClassid {
		t.Errorf("默认库应为总库, 实际 %s", opts.Classid)
	}
	if opts.Page != 1 {
		t.Errorf("默认页码应为 1, 实际 %d", opts.Page)
	}
	if opts.PageSize != 20 {
		t.Errorf("默认每页条数应为 20, 实际 %d", opts.PageSize)
	}
	if opts.SearchFrom != taxonomy.FromSearch {
		t.Errorf("默认检索来源应为 %d, 实际 %d", taxonomy.FromSearch, opts.SearchFrom)
	}
	// 总库检索才需要跨库代码.
	if opts.KuaKuCode != taxonomy.TotalLibraryKuaKuCode {
		t.Error("总库检索应带上跨库代码")
	}
}

// TestSingleDatabaseOmitsKuaKuCode 校验指定具体库时不带跨库代码.
//
// 实测这是单库检索能否成功的关键: 带上总库的跨库列表会让知网把请求判为
// 无效, 结果从二十余万条变成零条.
func TestSingleDatabaseOmitsKuaKuCode(t *testing.T) {
	db, ok := taxonomy.LookupDatabase("学位论文")
	if !ok {
		t.Fatal("学位论文库应当存在")
	}

	q, err := SimpleQuery("深度学习", "SU", SearchOptions{
		Classid:  db.Classid,
		Resource: db.Resource,
	})
	if err != nil {
		t.Fatalf("构造检索式失败: %v", err)
	}
	payload := q.QueryJSON()
	if payload.KuaKuCode != "" {
		t.Errorf("单库检索的 KuaKuCode 必须为空, 实际 %q", payload.KuaKuCode)
	}
	if payload.Classid != db.Classid {
		t.Errorf("Classid 应为 %s, 实际 %s", db.Classid, payload.Classid)
	}
	if payload.Resource != db.Resource {
		t.Errorf("Resource 应为 %s, 实际 %s", db.Resource, payload.Resource)
	}
}

// TestEveryDatabaseCanBuildQuery 校验所有已知库都能构造出请求.
func TestEveryDatabaseCanBuildQuery(t *testing.T) {
	for _, db := range taxonomy.Databases {
		q, err := SimpleQuery("深度学习", "SU", SearchOptions{
			Classid:  db.Classid,
			Resource: db.Resource,
		})
		if err != nil {
			t.Errorf("库 %s 构造检索式失败: %v", db.Name, err)
			continue
		}
		payload := q.QueryJSON()
		// 除总库外都不应携带跨库代码.
		wantKuaKu := ""
		if db.Classid == taxonomy.TotalLibraryClassid {
			wantKuaKu = taxonomy.TotalLibraryKuaKuCode
		}
		if payload.KuaKuCode != wantKuaKu {
			t.Errorf("库 %s 的 KuaKuCode 应为 %q, 实际 %q",
				db.Name, wantKuaKu, payload.KuaKuCode)
		}
	}
}

// TestGroupIDResolution 校验分组维度别名解析.
func TestGroupIDResolution(t *testing.T) {
	ids, err := ResolveGroupIDs([]string{"year", "discipline"})
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	want := GroupIDYear + "," + GroupIDDiscipline
	if ids != want {
		t.Errorf("分组 ID 拼装错误, 期望 %s, 实际 %s", want, ids)
	}

	// 原始 ID 也应被接受, 便于知网新增维度时无需改代码.
	if _, err := ResolveGroupIDs([]string{"ZYZT|||CYZT"}); err != nil {
		t.Errorf("原始 groupId 应被接受: %v", err)
	}

	if _, err := ResolveGroupIDs([]string{"不存在的维度"}); err == nil {
		t.Error("未知分组维度应当报错")
	}

	ids, err = ResolveGroupIDs(nil)
	if err != nil || ids != DefaultGroupIDs {
		t.Errorf("未指定分组时应使用默认值, 实际 %v %v", ids, err)
	}
}
