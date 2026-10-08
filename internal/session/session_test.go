package session

import (
	"testing"
	"time"
)

func TestParseCookieHeader(t *testing.T) {
	raw := "SID_kns_new=kns0000001; Ecp_ClientId=__abcdEF__0000000000000000000000000000; drlang=CHINESE"
	cookies, err := ParseCookieHeader(raw)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(cookies) != 3 {
		t.Fatalf("应解析出 3 项, 实际 %d 项", len(cookies))
	}

	sess := New("test", cookies)
	if got := sess.Value("drlang"); got != "CHINESE" {
		t.Errorf("取值错误: %q", got)
	}
	// CookieHeader 应还原成可直接放进请求头的形式.
	header := sess.CookieHeader()
	if header == "" {
		t.Fatal("CookieHeader 为空")
	}
	for _, c := range cookies {
		if !contains(header, c.Name+"="+c.Value) {
			t.Errorf("CookieHeader 缺少 %s", c.Name)
		}
	}
}

func TestParseCookieHeaderFromCurlStyleLine(t *testing.T) {
	// 容忍整行 "Cookie: a=1; b=2" 的写法, 方便直接粘贴 DevTools 内容.
	cookies, err := ParseCookieHeader("Cookie: a=1; b=2")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(cookies) != 2 {
		t.Fatalf("应解析出 2 项, 实际 %d 项", len(cookies))
	}
	if cookies[0].Name != "a" || cookies[0].Value != "1" {
		t.Errorf("解析结果错误: %+v", cookies[0])
	}
}

func TestParseCookieHeaderRejectsGarbage(t *testing.T) {
	if _, err := ParseCookieHeader(""); err == nil {
		t.Error("空内容应报错")
	}
	if _, err := ParseCookieHeader("这不是 cookie"); err == nil {
		t.Error("无等号的内容应报错")
	}
}

func TestParseAccount(t *testing.T) {
	// Ecp_LoginStuts 的取值是一个 JSON 字符串, 其中的机构名做过一次
	// 百分号编码, 解析后应还原成可读文本.
	// 下列取值均为构造的样例, 不代表任何真实账号.
	raw := `{"IsAutoLogin":false,"UserName":"user0001",` +
		`"ShowName":"%E7%A4%BA%E4%BE%8B%E5%A4%A7%E5%AD%A6",` +
		`"UserType":"bk","IsRealName":0}`
	sess := New("test", []Cookie{{Name: "Ecp_LoginStuts", Value: raw}})

	if !sess.Account.LoggedIn() {
		t.Fatal("应识别为已登录")
	}
	if sess.Account.UserName != "user0001" {
		t.Errorf("用户名解析错误: %q", sess.Account.UserName)
	}
	if sess.Account.ShowName != "示例大学" {
		t.Errorf("机构名未正确解码: %q", sess.Account.ShowName)
	}
}

func TestParseAccountAnonymous(t *testing.T) {
	// 只过了滑块但未登录个人账号时不应误报账号信息.
	sess := New("test", []Cookie{{Name: "SID_kns_new", Value: "kns123"}})
	if sess.Account.LoggedIn() {
		t.Errorf("匿名会话不应有账号信息: %+v", sess.Account)
	}
}

func TestNormalizeDeduplicates(t *testing.T) {
	cookies := Normalize([]Cookie{
		{Name: "b", Value: "2"},
		{Name: "a", Value: "1"},
		{Name: "b", Value: "3"},
		{Name: "", Value: "x"},
	})
	if len(cookies) != 2 {
		t.Fatalf("应去重且丢弃空名, 实际 %d 项", len(cookies))
	}
	// 结果应稳定排序, 便于会话文件比对.
	if cookies[0].Name != "a" || cookies[1].Name != "b" {
		t.Errorf("排序不稳定: %+v", cookies)
	}
	// 同名项后出现的应覆盖先出现的.
	if cookies[1].Value != "3" {
		t.Errorf("同名 cookie 应取后者: %+v", cookies[1])
	}
}

func TestExpiresHint(t *testing.T) {
	soon := time.Now().Add(10 * time.Minute)
	later := time.Now().Add(2 * time.Hour)
	sess := New("test", []Cookie{
		{Name: "a", Value: "1", Expires: later},
		{Name: "b", Value: "2", Expires: soon},
		{Name: "c", Value: "3"},
	})
	hint := sess.ExpiresHint()
	if hint.IsZero() {
		t.Fatal("应给出过期提示")
	}
	// 应取最近的一个过期时间.
	if hint.Sub(soon).Abs() > time.Second {
		t.Errorf("应取最早的过期时间, 实际 %v", hint)
	}
}

func TestMigrateSession(t *testing.T) {
	sess := &Session{SchemaVersion: 0}
	if err := Migrate(sess); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	if sess.SchemaVersion != CurrentSchemaVersion {
		t.Errorf("迁移后版本应为 %d, 实际 %d", CurrentSchemaVersion, sess.SchemaVersion)
	}

	// 来自未来版本的会话必须被拒绝, 而不是带着错误理解继续跑.
	future := &Session{SchemaVersion: CurrentSchemaVersion + 1}
	if err := Migrate(future); err == nil {
		t.Error("高于当前版本的会话应报错")
	}

	// nil 会话不应 panic.
	if err := Migrate(nil); err != nil {
		t.Errorf("nil 会话不应报错: %v", err)
	}
}

func TestEmptySession(t *testing.T) {
	var nilSession *Session
	if !nilSession.Empty() {
		t.Error("nil 会话应判定为空")
	}
	if nilSession.CookieHeader() != "" {
		t.Error("空会话的 CookieHeader 应为空串")
	}
	if nilSession.Value("any") != "" {
		t.Error("空会话取值应为空串")
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

func TestNormalizeCDPURL(t *testing.T) {
	cases := map[string]string{
		// 纯端口号必须补上主机, 这是最容易写错的一种写法.
		"9222":                     "http://127.0.0.1:9222",
		"":                         "http://127.0.0.1:9222",
		"127.0.0.1:9223":           "http://127.0.0.1:9223",
		"127.0.0.1":                "http://127.0.0.1:9222",
		"http://127.0.0.1:9224":    "http://127.0.0.1:9224",
		"http://localhost":         "http://localhost:9222",
		"http://192.168.1.10:9225": "http://192.168.1.10:9225",
	}
	for input, want := range cases {
		if got := NormalizeCDPURL(input); got != want {
			t.Errorf("NormalizeCDPURL(%q) = %q, 期望 %q", input, got, want)
		}
	}
}

func TestExpiresHintIgnoresIdentityCookies(t *testing.T) {
	// Ecp_ClientId 的有效期长达数百天, 但它只是客户端标识,
	// 不能用来判断服务端会话的寿命.
	sess := New("test", []Cookie{
		{Name: "Ecp_ClientId", Value: "abc", Expires: time.Now().Add(400 * 24 * time.Hour)},
		{Name: "fingerprint", Value: "def", Expires: time.Now().Add(300 * 24 * time.Hour)},
		{Name: "SID_kns_new", Value: "kns1"},
	})
	if hint := sess.ExpiresHint(); !hint.IsZero() {
		t.Errorf("只有身份类 cookie 时不应给出会话期限, 实际 %v", hint)
	}

	// 真正反映会话寿命的 cookie 出现时仍应被采用.
	soon := time.Now().Add(20 * time.Minute)
	sess.Cookies = append(sess.Cookies, Cookie{Name: "LID", Value: "x", Expires: soon})
	hint := sess.ExpiresHint()
	if hint.IsZero() {
		t.Fatal("应给出会话期限")
	}
	if hint.Sub(soon).Abs() > time.Second {
		t.Errorf("应取 LID 的过期时间, 实际 %v", hint)
	}
}

func TestHumanDuration(t *testing.T) {
	cases := map[time.Duration]string{
		0:                    "已过期",
		-1 * time.Hour:       "已过期",
		30 * time.Second:     "30 秒",
		5 * time.Minute:      "5 分钟",
		3 * time.Hour:        "3 小时",
		72 * time.Hour:       "3 天",
		400 * 24 * time.Hour: "400 天",
	}
	for input, want := range cases {
		if got := humanDuration(input); got != want {
			t.Errorf("humanDuration(%v) = %q, 期望 %q", input, got, want)
		}
	}
}
