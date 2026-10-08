package cli

import (
	"os"
	"strings"
	"unicode/utf8"

	"github.com/azazo1/cnki-cli/internal/apperr"
	"github.com/azazo1/cnki-cli/internal/taxonomy"
)

// maxFileNameBytes 是生成文件名的字节上限.
//
// 必须按字节而不是字符数限制: 多数文件系统的上限是 255 字节, 而中文一个字
// 占三个字节, 按字符数限长会让中文标题轻易越界. 这里留出扩展名与重名后缀
// 的余量.
const maxFileNameBytes = 200

// resolveExportFormat 解析导出格式参数.
func resolveExportFormat(name string) (taxonomy.ExportFormat, error) {
	spec, ok := taxonomy.LookupExportFormat(name)
	if !ok {
		return taxonomy.ExportFormat{}, apperr.Usage("未知的导出格式 %q, 可用值见 cnki info formats", name)
	}
	return spec, nil
}

// createFile 创建输出文件.
func createFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
}

// sanitizeFileName 把文献标题转成可安全落盘的文件名.
//
// 标题里常见斜杠, 冒号, 问号等在各平台上有特殊含义的字符, 还可能出现换行,
// 直接当文件名会写失败或写到意想不到的位置, 因此统一替换掉.
func sanitizeFileName(title string) string {
	var b strings.Builder
	for _, r := range title {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			// 换行与制表符换成空格, 免得相邻的词语被粘在一起.
			b.WriteRune(' ')
		case r < 0x20 || r == 0x7F:
			// 其余控制字符直接丢弃.
		case strings.ContainsRune(`<>:"/\|?*`, r):
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}

	// 折叠连续空白, 并去掉首尾的点与空格: Windows 会静默去掉结尾的点,
	// 留着会导致写出的文件名与预期不符.
	name := strings.Join(strings.Fields(b.String()), " ")
	name = strings.Trim(name, " .")
	if name == "" {
		return "cnki-fulltext"
	}

	if len(name) > maxFileNameBytes {
		name = truncateUTF8(name, maxFileNameBytes)
	}
	name = strings.TrimRight(name, " .")
	if name == "" {
		return "cnki-fulltext"
	}
	return name
}

// truncateUTF8 把字符串截到不超过 limit 字节, 且不切开最后一个字符.
func truncateUTF8(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut]
}
