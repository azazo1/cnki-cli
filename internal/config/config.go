package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// CurrentSchemaVersion 是当前代码期望的配置版本号.
//
// 任何不兼容的结构调整都必须递增它, 并在 migrate.go 中补上对应的升级步骤.
const CurrentSchemaVersion = 1

// Config 是完整的配置结构.
type Config struct {
	// SchemaVersion 用于判断是否需要迁移旧配置.
	SchemaVersion int `toml:"schema_version"`

	Request RequestConfig `toml:"request"`
	Output  OutputConfig  `toml:"output"`
	Browser BrowserConfig `toml:"browser"`
	Session SessionConfig `toml:"session"`
}

// RequestConfig 描述 HTTP 请求行为.
type RequestConfig struct {
	// IntervalMs 是相邻两次请求之间的最小间隔, 用于规避知网限流.
	IntervalMs int `toml:"interval_ms"`
	// TimeoutSec 是单次请求超时.
	TimeoutSec int `toml:"timeout_sec"`
	// Retries 是失败重试次数.
	Retries int `toml:"retries"`
	// UserAgent 是请求使用的浏览器标识, 留空使用内置默认值.
	UserAgent string `toml:"user_agent"`
	// BaseURL 是知网主站地址, 一般无需修改.
	BaseURL string `toml:"base_url"`
}

// OutputConfig 描述默认输出行为.
type OutputConfig struct {
	// Format 是默认输出格式, 取值为 table / json / csv / markdown.
	Format string `toml:"format"`
	// PageSize 是每次向知网请求的记录条数.
	PageSize int `toml:"page_size"`
	// Limit 是单条命令默认取回的记录上限, 0 表示不限制.
	Limit int `toml:"limit"`
}

// BrowserConfig 描述受控浏览器接管行为.
type BrowserConfig struct {
	// ChromePath 是 Chrome 可执行文件路径, 留空自动探测.
	ChromePath string `toml:"chrome_path"`
	// Headless 控制是否无头启动, 需要人工过验证时必须为 false.
	Headless bool `toml:"headless"`
	// VerifyTimeoutSec 是等待人工完成验证的超时.
	VerifyTimeoutSec int `toml:"verify_timeout_sec"`
	// CDPURL 指向一个已开启远程调试的浏览器实例.
	//
	// 配置后登录会直接接管该实例而不启动新浏览器, 适合日常浏览器里已经
	// 登录过知网的场景.
	CDPURL string `toml:"cdp_url"`
}

// SessionConfig 描述会话失效策略.
type SessionConfig struct {
	// OnExpired 取值 relogin / fail / prompt, 决定会话失效时的行为.
	OnExpired string `toml:"on_expired"`
}

// Default 返回内置默认配置.
func Default() Config {
	return Config{
		SchemaVersion: CurrentSchemaVersion,
		Request: RequestConfig{
			IntervalMs: 1500,
			TimeoutSec: 30,
			Retries:    2,
			BaseURL:    "https://kns.cnki.net",
		},
		Output: OutputConfig{
			Format:   "table",
			PageSize: 20,
			Limit:    20,
		},
		Browser: BrowserConfig{
			Headless:         false,
			VerifyTimeoutSec: 300,
		},
		Session: SessionConfig{
			OnExpired: "relogin",
		},
	}
}

// Interval 把 IntervalMs 转为 time.Duration.
func (r RequestConfig) Interval() time.Duration {
	if r.IntervalMs <= 0 {
		return 0
	}
	return time.Duration(r.IntervalMs) * time.Millisecond
}

// Timeout 把 TimeoutSec 转为 time.Duration.
func (r RequestConfig) Timeout() time.Duration {
	if r.TimeoutSec <= 0 {
		return 30 * time.Second
	}
	return time.Duration(r.TimeoutSec) * time.Second
}

// VerifyTimeout 把 VerifyTimeoutSec 转为 time.Duration.
func (b BrowserConfig) VerifyTimeout() time.Duration {
	if b.VerifyTimeoutSec <= 0 {
		return 5 * time.Minute
	}
	return time.Duration(b.VerifyTimeoutSec) * time.Second
}

// Load 读取配置. 文件不存在时生成默认配置并落盘, 返回默认值.
//
// 读到旧版本配置时先执行迁移, 迁移结果写回磁盘后才返回.
func Load() (Config, error) {
	path := ConfigPath()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		cfg := Default()
		if saveErr := Save(cfg); saveErr != nil {
			return cfg, fmt.Errorf("写入默认配置 %s: %w", path, saveErr)
		}
		return cfg, nil
	}
	if err != nil {
		return Default(), fmt.Errorf("读取配置 %s: %w", path, err)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Default(), fmt.Errorf("解析配置 %s: %w", path, err)
	}

	migrated, changed, err := Migrate(cfg)
	if err != nil {
		return Default(), fmt.Errorf("迁移配置 %s: %w", path, err)
	}
	if changed {
		if err := Save(migrated); err != nil {
			return migrated, fmt.Errorf("写回迁移后的配置 %s: %w", path, err)
		}
	}
	return migrated.withDefaults(), nil
}

// Save 把配置写入磁盘.
func Save(cfg Config) error {
	path := ConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// withDefaults 为缺省字段补上默认值, 避免手工编辑出的半成品配置导致零值行为.
func (c Config) withDefaults() Config {
	def := Default()
	if c.SchemaVersion == 0 {
		c.SchemaVersion = CurrentSchemaVersion
	}
	if c.Request.IntervalMs == 0 {
		c.Request.IntervalMs = def.Request.IntervalMs
	}
	if c.Request.TimeoutSec == 0 {
		c.Request.TimeoutSec = def.Request.TimeoutSec
	}
	if c.Request.BaseURL == "" {
		c.Request.BaseURL = def.Request.BaseURL
	}
	if c.Output.Format == "" {
		c.Output.Format = def.Output.Format
	}
	if c.Output.PageSize == 0 {
		c.Output.PageSize = def.Output.PageSize
	}
	if c.Browser.VerifyTimeoutSec == 0 {
		c.Browser.VerifyTimeoutSec = def.Browser.VerifyTimeoutSec
	}
	if c.Session.OnExpired == "" {
		c.Session.OnExpired = def.Session.OnExpired
	}
	return c
}
