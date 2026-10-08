package cnki

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/azazo1/cnki-cli/internal/config"
	"github.com/azazo1/cnki-cli/internal/taxonomy"
)

// testCacheDir 把缓存目录指到临时目录, 让涉及落盘的测试彼此隔离.
func testCacheDir(t *testing.T) {
	t.Helper()
	t.Setenv(config.EnvCacheDir, t.TempDir())
}

func TestRefineStructuredAppendsAndCondition(t *testing.T) {
	base, err := SimpleQuery("大语言模型", "", SearchOptions{})
	if err != nil {
		t.Fatalf("构造基座失败: %v", err)
	}

	refined, err := base.Refine([]Condition{{Field: "作者", Value: "佘维"}}, "")
	if err != nil {
		t.Fatalf("收窄失败: %v", err)
	}

	items := refined.QueryJSON().QNode.QGroup[0].Items
	if len(items) != 2 {
		t.Fatalf("应在原检索项后追加一项, 实际 %d 项", len(items))
	}
	if items[1].Field != "AU" || items[1].Value != "佘维" {
		t.Errorf("追加的检索项不正确: %+v", items[1])
	}
	// 原检索项与新增项之间必须收敛为 AND.
	if items[0].Logic != int(taxonomy.LogicAnd) || items[1].Logic != int(taxonomy.LogicAnd) {
		t.Errorf("追加后应为 AND 连接, 实际 %d 与 %d", items[0].Logic, items[1].Logic)
	}
	// 来源要标记为结果中检索, 让知网的检索历史归类正确.
	if got := refined.QueryJSON().SearchFrom; got != int(taxonomy.FromResult) {
		t.Errorf("SearchFrom 应为 %d, 实际 %d", taxonomy.FromResult, got)
	}
	// 基座不应被改动, 否则连续调用会不断累积条件.
	if len(base.QueryJSON().QNode.QGroup[0].Items) != 1 {
		t.Error("收窄不应改动原来的检索式")
	}
}

func TestRefineForcesAndRegardlessOfInputLogic(t *testing.T) {
	base, err := SimpleQuery("深度学习", "", SearchOptions{})
	if err != nil {
		t.Fatalf("构造基座失败: %v", err)
	}
	// 即使调用方给了 OR, 收窄也应当按 AND 处理.
	refined, err := base.Refine([]Condition{
		{Field: "作者", Value: "张三", Logic: taxonomy.LogicOr},
	}, "")
	if err != nil {
		t.Fatalf("收窄失败: %v", err)
	}
	items := refined.QueryJSON().QNode.QGroup[0].Items
	if items[1].Logic != int(taxonomy.LogicAnd) {
		t.Errorf("收窄条件的逻辑应为 AND, 实际 %d", items[1].Logic)
	}
}

func TestRefineExpertConcatenatesExpression(t *testing.T) {
	base, err := ExpertQuery("SU=('深度学习')", SearchOptions{})
	if err != nil {
		t.Fatalf("构造基座失败: %v", err)
	}

	refined, err := base.Refine([]Condition{{Field: "文献来源", Value: "计算机学报"}}, "")
	if err != nil {
		t.Fatalf("收窄失败: %v", err)
	}

	groupIndex, itemIndex := refined.expertItemIndex()
	if groupIndex < 0 {
		t.Fatal("收窄后找不到专业检索表达式")
	}
	got := refined.groups[groupIndex].Items[itemIndex].Value

	// 原表达式必须被括起来, 否则拼出的优先级会改变.
	if !strings.HasPrefix(got, "(SU=('深度学习')) AND ") {
		t.Errorf("表达式拼接形式不正确: %q", got)
	}
	// 文献来源的默认匹配是模糊, 应翻译成百分号.
	if !strings.Contains(got, "LY%('计算机学报')") {
		t.Errorf("模糊字段应翻译成百分号: %q", got)
	}
}

func TestRefineExpertAcceptsRawExpression(t *testing.T) {
	base, err := ExpertQuery("SU=('深度学习')", SearchOptions{})
	if err != nil {
		t.Fatalf("构造基座失败: %v", err)
	}
	refined, err := base.Refine(nil, "LY='计算机学报'")
	if err != nil {
		t.Fatalf("收窄失败: %v", err)
	}
	groupIndex, itemIndex := refined.expertItemIndex()
	got := refined.groups[groupIndex].Items[itemIndex].Value
	if !strings.Contains(got, "(LY='计算机学报')") {
		t.Errorf("追加的表达式应被括起来: %q", got)
	}
}

// TestRefineRejectsExpressionOnStructuredBase 校验结构化检索式不接受裸表达式.
//
// 把表达式片段混进结构化检索组会引入不确定的匹配语义, 因此宁可拒绝并
// 指出正确做法, 也不猜.
func TestRefineRejectsExpressionOnStructuredBase(t *testing.T) {
	base, err := SimpleQuery("深度学习", "", SearchOptions{})
	if err != nil {
		t.Fatalf("构造基座失败: %v", err)
	}
	if _, err := base.Refine(nil, "AU='张三'"); err == nil {
		t.Fatal("结构化检索式应当拒绝 --expr")
	}
}

func TestRefineRequiresCondition(t *testing.T) {
	base, err := SimpleQuery("深度学习", "", SearchOptions{})
	if err != nil {
		t.Fatalf("构造基座失败: %v", err)
	}
	if _, err := base.Refine(nil, ""); err == nil {
		t.Fatal("没有收窄条件时应当报错")
	}
	if _, err := base.Refine([]Condition{{Field: "作者", Value: "  "}}, ""); err == nil {
		t.Fatal("检索词为空时应当报错")
	}
}

func TestQuoteExpertValueSwitchesQuoteStyle(t *testing.T) {
	// 检索词自身含单引号时改用双引号, 避免拼出语法错误的表达式.
	if got := quoteExpertValue("it's"); got != `"it's"` {
		t.Errorf("含单引号的检索词应用双引号, 实际 %q", got)
	}
	if got := quoteExpertValue("深度学习"); got != "'深度学习'" {
		t.Errorf("普通检索词应用单引号, 实际 %q", got)
	}
}

func TestLastQueryRoundTrip(t *testing.T) {
	testCacheDir(t)

	base, err := AdvancedQuery([]Condition{
		{Field: "篇名", Value: "大语言模型"},
		{Field: "基金", Value: "国家自然科学基金", Logic: taxonomy.LogicAnd},
	}, SearchOptions{})
	if err != nil {
		t.Fatalf("构造检索式失败: %v", err)
	}
	if err := SaveLastQuery(base); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	loaded, err := LoadLastQuery()
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}

	if loaded.Description() != base.Description() {
		t.Errorf("检索式描述未还原: %q vs %q", loaded.Description(), base.Description())
	}
	if loaded.SearchType() != base.SearchType() {
		t.Errorf("检索方式未还原: %v vs %v", loaded.SearchType(), base.SearchType())
	}
	// 还原出来的检索式必须能再次提交, 且条件与原来一致.
	original := base.QueryJSON()
	restored := loaded.QueryJSON()
	if len(restored.QNode.QGroup) != len(original.QNode.QGroup) {
		t.Fatalf("检索组数量不一致: %d vs %d",
			len(restored.QNode.QGroup), len(original.QNode.QGroup))
	}
	if len(restored.QNode.QGroup[0].Items) != 2 {
		t.Errorf("检索项数量不一致: %d", len(restored.QNode.QGroup[0].Items))
	}
	if restored.Classid != original.Classid {
		t.Errorf("库标识未还原: %q vs %q", restored.Classid, original.Classid)
	}
}

func TestLoadLastQueryWithoutRecord(t *testing.T) {
	testCacheDir(t)

	_, err := LoadLastQuery()
	if err == nil {
		t.Fatal("没有记录时应当报错")
	}
	// 提示要能直接照做, 而不是只说文件不存在.
	if !strings.Contains(err.Error(), "先执行一次检索") {
		t.Errorf("错误提示应指出下一步怎么做, 实际: %v", err)
	}
}

func TestLoadLastQueryBrokenFile(t *testing.T) {
	testCacheDir(t)

	path := LastQueryPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("准备目录失败: %v", err)
	}
	if err := os.WriteFile(path, []byte("{ 不是 JSON"), 0o600); err != nil {
		t.Fatalf("准备坏文件失败: %v", err)
	}
	if _, err := LoadLastQuery(); err == nil {
		t.Fatal("内容损坏时应当报错")
	}
}

func TestClearLastQuery(t *testing.T) {
	testCacheDir(t)

	if err := SaveLastQuery(mustSimple(t, "深度学习")); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	if err := ClearLastQuery(); err != nil {
		t.Fatalf("清除失败: %v", err)
	}
	// 已经不存在时清除也不应报错.
	if err := ClearLastQuery(); err != nil {
		t.Fatalf("重复清除不应报错: %v", err)
	}
	if _, err := LoadLastQuery(); err == nil {
		t.Error("清除后应读不到记录")
	}
}

func TestTruthyHandlesBothShapes(t *testing.T) {
	// ExScope 在不同检索方式下分别是数字与字符串, 两种都要认.
	cases := []struct {
		value any
		want  bool
	}{
		{float64(1), true},
		{float64(0), false},
		{1, true},
		{0, false},
		{"1", true},
		{"0", false},
		{"", false},
		{"false", false},
		{true, true},
		{false, false},
		{nil, false},
	}
	for _, c := range cases {
		if got := truthy(c.value); got != c.want {
			t.Errorf("truthy(%#v) = %v, 期望 %v", c.value, got, c.want)
		}
	}
}

func mustSimple(t *testing.T, keyword string) *Query {
	t.Helper()
	q, err := SimpleQuery(keyword, "", SearchOptions{})
	if err != nil {
		t.Fatalf("构造检索式失败: %v", err)
	}
	return q
}
