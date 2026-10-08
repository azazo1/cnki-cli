// Package taxonomy 收录知网的各类枚举常量.
//
// 这些取值全部来自对知网页面与前端脚本的实测提取, 而非文档猜测.
// 知网改版时只需在这里修正, 无需改动业务代码.
package taxonomy

import "strings"

// Database 描述知网的一个文献库.
type Database struct {
	// Name 是中文库名, 与知网界面一致.
	Name string
	// Classid 是知网内部库标识, 作为检索请求的 Classid 参数.
	Classid string
	// Resource 是资源类型标识, 与 Classid 一一对应但语义更粗.
	Resource string
	// Group 是归一化后的库族, 用于把"博士/硕士"归到"学位论文"下.
	Group string
}

// 库族名, 用于 Group 字段.
const (
	GroupCrossDB      = "crossdb"
	GroupJournal      = "journal"
	GroupDissertation = "dissertation"
	GroupConference   = "conference"
	GroupNewspaper    = "newspaper"
	GroupAlmanac      = "almanac"
	GroupPatent       = "patent"
	GroupStandard     = "standard"
	GroupBook         = "book"
	GroupAchievement  = "achievement"
	GroupLaw          = "law"
	GroupVideo        = "video"
	GroupDocument     = "document"
	GroupOther        = "other"
)

// Databases 是知网全部文献库, 顺序与网页顶部库菜单一致.
var Databases = []Database{
	{Name: "总库", Classid: "WD0FTY92", Resource: "CROSSDB", Group: GroupCrossDB},
	{Name: "学术期刊", Classid: "YSTT4HG0", Resource: "JOURNAL", Group: GroupJournal},
	{Name: "学位论文", Classid: "LSTPFY1C", Resource: "DISSERTATION", Group: GroupDissertation},
	{Name: "博士", Classid: "RMJLXHZ3", Resource: "DISSERTATION", Group: GroupDissertation},
	{Name: "硕士", Classid: "JQIRZIYA", Resource: "DISSERTATION", Group: GroupDissertation},
	{Name: "图书", Classid: "EMRPGLPA", Resource: "BOOK", Group: GroupBook},
	{Name: "中文图书", Classid: "J708GVCE", Resource: "BOOK", Group: GroupBook},
	{Name: "外文图书", Classid: "ML4DRIDX", Resource: "BOOK", Group: GroupBook},
	{Name: "会议", Classid: "JUP3MUPD", Resource: "CONFERENCE", Group: GroupConference},
	{Name: "国内会议", Classid: "1UR4K4HZ", Resource: "CONFERENCE", Group: GroupConference},
	{Name: "国际会议", Classid: "BPBAFJ5S", Resource: "CONFERENCE", Group: GroupConference},
	{Name: "外文会议", Classid: "4OG3ORSB", Resource: "CONFERENCE", Group: GroupConference},
	{Name: "报纸", Classid: "MPMFIG1A", Resource: "NEWSPAPER", Group: GroupNewspaper},
	{Name: "年鉴", Classid: "HHCPM1F8", Resource: "ALMANAC", Group: GroupAlmanac},
	{Name: "专利", Classid: "VUDIXAIY", Resource: "PATENT", Group: GroupPatent},
	{Name: "中国专利", Classid: "OORPU5FE", Resource: "PATENT", Group: GroupPatent},
	{Name: "境外专利", Classid: "U8J8LYLV", Resource: "PATENT", Group: GroupPatent},
	{Name: "标准", Classid: "WQ0UVIAA", Resource: "STANDARD", Group: GroupStandard},
	{Name: "国家标准", Classid: "NB3BWEHK", Resource: "STANDARD", Group: GroupStandard},
	{Name: "行业标准", Classid: "XVLO76FD", Resource: "STANDARD", Group: GroupStandard},
	{Name: "标准题录", Classid: "HR1YT1Z9", Resource: "STANDARD", Group: GroupStandard},
	{Name: "企业标准", Classid: "QZCO9HKB", Resource: "", Group: GroupStandard},
	{Name: "法律法规", Classid: "8JBZLDJQ", Resource: "LAW_STATUTE", Group: GroupLaw},
	{Name: "成果", Classid: "BLZOG7CK", Resource: "ACHIEVEMENTS", Group: GroupAchievement},
	{Name: "学术辑刊", Classid: "PWFIRAGL", Resource: "JOURNAL", Group: GroupJournal},
	{Name: "学术图片", Classid: "CP6HP56B", Resource: "", Group: GroupOther},
	{Name: "科技报告", Classid: "G9R1H3GF", Resource: "", Group: GroupOther},
	{Name: "政府采购", Classid: "N779QGNL", Resource: "", Group: GroupOther},
	{Name: "工具书", Classid: "ZZJR8PBZ", Resource: "", Group: GroupOther},
	{Name: "特色期刊", Classid: "NN3FJMUV", Resource: "JOURNAL", Group: GroupJournal},
	{Name: "视频", Classid: "NLBO1Z6R", Resource: "VIDEO", Group: GroupVideo},
	{Name: "文库", Classid: "T2VC03OH", Resource: "DOCUMENT", Group: GroupDocument},
	{Name: "政府文件", Classid: "O2RXINIV", Resource: "", Group: GroupOther},
}

// TotalLibraryClassid 是总库的 Classid, 跨库检索时使用.
const TotalLibraryClassid = "WD0FTY92"

// TotalLibraryKuaKuCode 是"总库"检索时使用的默认跨库列表.
//
// 来源: 知网页面 #CheckedDB 隐藏域的初始值, 共 10 个库.
const TotalLibraryKuaKuCode = "YSTT4HG0,LSTPFY1C,EMRPGLPA,JUP3MUPD,MPMFIG1A," +
	"WQ0UVIAA,BLZOG7CK,PWFIRAGL,NN3FJMUV,NLBO1Z6R"

// LookupDatabase 按中文库名, Classid 或 Resource 标识查找库.
//
// 查找不区分大小写, 并容忍首尾空白.
func LookupDatabase(key string) (Database, bool) {
	key = strings.TrimSpace(key)
	if key == "" {
		return Database{}, false
	}
	upper := strings.ToUpper(key)
	for _, db := range Databases {
		if db.Name == key || strings.EqualFold(db.Classid, key) {
			return db, true
		}
		if db.Resource != "" && db.Resource == upper {
			return db, true
		}
	}
	return Database{}, false
}

// DatabasesByGroup 返回某个库族下的全部库.
func DatabasesByGroup(group string) []Database {
	var out []Database
	for _, db := range Databases {
		if db.Group == group {
			out = append(out, db)
		}
	}
	return out
}
