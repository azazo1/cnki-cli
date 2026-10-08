package session

import "fmt"

// Migrate 把任意历史版本的会话升级到 CurrentSchemaVersion.
//
// 会话里混着旧版本 cookie 时不必惊慌: 知网侧看的是 cookie 内容是否仍然有效,
// 版本号只用于在结构变化时正确解释字段.
func Migrate(s *Session) error {
	if s == nil {
		return nil
	}
	if s.SchemaVersion > CurrentSchemaVersion {
		return fmt.Errorf(
			"会话版本 %d 高于本程序支持的 %d, 请升级 cnki-cli",
			s.SchemaVersion, CurrentSchemaVersion)
	}
	for s.SchemaVersion < CurrentSchemaVersion {
		switch s.SchemaVersion {
		case 0:
			// 早期会话文件没有版本号字段, 直接归入当前结构.
			s.SchemaVersion = 1
		default:
			return fmt.Errorf("缺少从版本 %d 升级的会话迁移步骤", s.SchemaVersion)
		}
	}
	return nil
}
