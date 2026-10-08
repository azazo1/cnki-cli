package cli

import (
	"os"

	"github.com/azazo1/cnki-cli/internal/apperr"
	"github.com/azazo1/cnki-cli/internal/taxonomy"
)

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
