package cli

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSanitizeFileName(t *testing.T) {
	cases := []struct{ name, input, want string }{
		{"普通标题", "基于深度学习的图像识别", "基于深度学习的图像识别"},
		{"斜杠", "A/B 研究", "A_B 研究"},
		{"冒号与问号", "问题:是什么?", "问题_是什么_"},
		{"反斜杠与竖线", `a\b|c`, "a_b_c"},
		{"换行与制表", "第一行\n第二行\t结束", "第一行 第二行 结束"},
		{"首尾空白", "  标题  ", "标题"},
		{"纯控制字符", "\x00\x01", "cnki-fulltext"},
		{"空标题", "", "cnki-fulltext"},
		{"只有点", "...", "cnki-fulltext"},
		{"Windows 尾部点", "标题...", "标题"},
		{"星号", "a*b", "a_b"},
	}
	for _, c := range cases {
		if got := sanitizeFileName(c.input); got != c.want {
			t.Errorf("%s: sanitizeFileName(%q) = %q, 期望 %q", c.name, c.input, got, c.want)
		}
	}
}

func TestSanitizeFileNameLimitsLength(t *testing.T) {
	// 中文一个字三个字节, 因此限长必须按字节做, 否则很短的标题就会越界.
	long := strings.Repeat("中", 300)
	got := sanitizeFileName(long)

	if len(got) > maxFileNameBytes {
		t.Errorf("截断后仍超过上限: %d 字节", len(got))
	}
	// 加上扩展名后仍要在文件系统的 255 字节之内.
	if len(got)+len(".pdf") > 255 {
		t.Errorf("加上扩展名后超过 255 字节: %d", len(got)+len(".pdf"))
	}
	// 截断不能切开一个字符, 否则会得到无效的 UTF-8.
	if !utf8.ValidString(got) {
		t.Error("截断后不是合法的 UTF-8")
	}
	if strings.HasSuffix(got, "\ufffd") {
		t.Error("截断处出现了替换字符")
	}
}

func TestSanitizeFileNameNoPathTraversal(t *testing.T) {
	// 标题里带的路径分隔符必须被替换, 否则会把文件写到别处.
	for _, input := range []string{"../../etc/passwd", `..\..\windows\system32`, "/绝对路径"} {
		got := sanitizeFileName(input)
		if strings.ContainsAny(got, `/\`) {
			t.Errorf("sanitizeFileName(%q) = %q, 仍含路径分隔符", input, got)
		}
		if strings.HasPrefix(got, "..") {
			t.Errorf("sanitizeFileName(%q) = %q, 仍以上级目录开头", input, got)
		}
	}
}
