package config

import (
	"testing"
	"time"
)

func TestDefaultConfigIsValid(t *testing.T) {
	cfg := Default()
	if cfg.SchemaVersion != CurrentSchemaVersion {
		t.Errorf("默认配置版本应为 %d", CurrentSchemaVersion)
	}
	if cfg.Request.Interval() <= 0 {
		t.Error("默认请求间隔必须为正, 否则容易触发知网限流")
	}
	if cfg.Request.Timeout() <= 0 {
		t.Error("默认超时必须为正")
	}
	if cfg.Browser.Headless {
		t.Error("默认必须是有头模式, 否则无法完成滑块验证")
	}
	if cfg.Session.OnExpired != "relogin" {
		t.Errorf("默认会话失效策略应为 relogin, 实际 %q", cfg.Session.OnExpired)
	}
}

func TestMigrateFromLegacyVersion(t *testing.T) {
	// 早期配置没有版本号字段, 解析出来是 0.
	legacy := Config{SchemaVersion: 0}
	migrated, changed, err := Migrate(legacy)
	if err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	if !changed {
		t.Error("迁移应报告结构已变更")
	}
	if migrated.SchemaVersion != CurrentSchemaVersion {
		t.Errorf("迁移后版本应为 %d, 实际 %d", CurrentSchemaVersion, migrated.SchemaVersion)
	}

	// 已是当前版本时不应报告变更.
	_, changed, err = Migrate(migrated)
	if err != nil {
		t.Fatalf("重复迁移失败: %v", err)
	}
	if changed {
		t.Error("版本一致时不应报告变更")
	}
}

func TestMigrateRejectsFutureVersion(t *testing.T) {
	future := Config{SchemaVersion: CurrentSchemaVersion + 1}
	if _, _, err := Migrate(future); err == nil {
		t.Error("高于当前版本的配置应报错, 而不是带着错误理解继续运行")
	}
}

func TestWithDefaultsFillsZeroValues(t *testing.T) {
	// 手工编辑出的半成品配置不应导致零值行为.
	partial := Config{SchemaVersion: CurrentSchemaVersion}
	filled := partial.withDefaults()

	if filled.Request.IntervalMs != Default().Request.IntervalMs {
		t.Errorf("请求间隔未补默认值: %d", filled.Request.IntervalMs)
	}
	if filled.Output.PageSize != Default().Output.PageSize {
		t.Errorf("每页条数未补默认值: %d", filled.Output.PageSize)
	}
	if filled.Session.OnExpired == "" {
		t.Error("会话失效策略未补默认值")
	}
}

func TestDurationHelpers(t *testing.T) {
	req := RequestConfig{IntervalMs: 1500, TimeoutSec: 30}
	if req.Interval() != 1500*time.Millisecond {
		t.Errorf("间隔换算错误: %v", req.Interval())
	}
	if req.Timeout() != 30*time.Second {
		t.Errorf("超时换算错误: %v", req.Timeout())
	}

	// 零值应回落到安全默认, 而不是 0 导致立即超时.
	zero := RequestConfig{}
	if zero.Timeout() <= 0 {
		t.Error("零值超时应回落到默认值")
	}
	if zero.Interval() != 0 {
		t.Error("零值间隔应表示不限速")
	}
}
