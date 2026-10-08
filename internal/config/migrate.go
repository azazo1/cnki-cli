package config

import "fmt"

// Migrate 把任意历史版本的配置升级到 CurrentSchemaVersion.
//
// 返回值 changed 表示配置结构被改动, 调用方需要把结果写回磁盘.
// 迁移按版本号逐级执行, 每一步只负责一个版本到下一个版本的差异,
// 因此新增版本时只需追加一个步骤, 无需修改既有步骤.
func Migrate(cfg Config) (Config, bool, error) {
	if cfg.SchemaVersion > CurrentSchemaVersion {
		return cfg, false, fmt.Errorf(
			"配置版本 %d 高于本程序支持的 %d, 请升级 cnki-cli",
			cfg.SchemaVersion, CurrentSchemaVersion)
	}

	original := cfg.SchemaVersion
	for cfg.SchemaVersion < CurrentSchemaVersion {
		switch cfg.SchemaVersion {
		case 0:
			// 0 表示早期无 Version 字段的配置, 直接标记为当前版本并交给
			// withDefaults 补全新增字段.
			cfg.SchemaVersion = 1
		default:
			return cfg, false, fmt.Errorf("缺少从版本 %d 升级的迁移步骤", cfg.SchemaVersion)
		}
	}
	return cfg, original != cfg.SchemaVersion, nil
}
