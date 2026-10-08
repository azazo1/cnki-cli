package taxonomy

import "strings"

// ExportFormat 描述一种题录导出格式.
type ExportFormat struct {
	// Name 是中文名.
	Name string
	// Alias 是命令行使用的英文别名.
	Alias string
	// DisplayMode 是知网导出接口 displaymode 参数的取值.
	//
	// 为空表示知网不提供该格式, 由本程序在本地从文献模型渲染生成.
	DisplayMode string
	// Local 表示该格式由本程序本地生成.
	Local bool
}

// ExportFormats 是全部支持的题录导出格式.
//
// DisplayMode 取值来自知网页面 displayModeArr 的实测值:
// GBTREFER, REFER, NEW, newdefine, elearning, Refworks, EndNote,
// NoteExpress, NodeFirst, selfDefine, MLA, APA.
//
// RIS 与 BibTeX 知网不提供, 因此由本程序本地渲染.
var ExportFormats = []ExportFormat{
	{Name: "GB/T 7714-2015", Alias: "gbt7714", DisplayMode: "GBTREFER"},
	{Name: "参考文献", Alias: "reference", DisplayMode: "REFER"},
	{Name: "知网通用", Alias: "cnki", DisplayMode: "NEW"},
	{Name: "MLA", Alias: "mla", DisplayMode: "MLA"},
	{Name: "APA", Alias: "apa", DisplayMode: "APA"},
	{Name: "RefWorks", Alias: "refworks", DisplayMode: "Refworks"},
	{Name: "EndNote", Alias: "endnote", DisplayMode: "EndNote"},
	{Name: "NoteExpress", Alias: "noteexpress", DisplayMode: "NoteExpress"},
	{Name: "NoteFirst", Alias: "notefirst", DisplayMode: "NodeFirst"},
	{Name: "自定义", Alias: "custom", DisplayMode: "selfDefine"},
	{Name: "RIS", Alias: "ris", Local: true},
	{Name: "BibTeX", Alias: "bibtex", Local: true},
}

// LookupExportFormat 按中文名或别名查找导出格式.
func LookupExportFormat(key string) (ExportFormat, bool) {
	key = strings.TrimSpace(key)
	if key == "" {
		return ExportFormats[0], true
	}
	for _, f := range ExportFormats {
		if f.Name == key || strings.EqualFold(f.Alias, key) {
			return f, true
		}
	}
	return ExportFormat{}, false
}
