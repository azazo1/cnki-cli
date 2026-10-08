// Package config 负责配置目录定位, 配置读写与配置版本迁移.
//
// 目录解析优先级: CNKI_CONFIG_DIR 环境变量 > 平台默认位置.
// 平台默认位置在类 Unix 系统上遵循 XDG 约定, 即 ~/.config/cnki-cli.
package config

import (
	"os"
	"path/filepath"
	"runtime"
)

const (
	// appDirName 是配置与缓存目录名.
	appDirName = "cnki-cli"

	// EnvConfigDir 用于覆盖配置目录, 测试与多账号隔离都靠它.
	EnvConfigDir = "CNKI_CONFIG_DIR"
	// EnvCacheDir 用于覆盖缓存目录, 浏览器独立 profile 也放在这里.
	EnvCacheDir = "CNKI_CACHE_DIR"
	// EnvCookie 用于直接提供知网 cookie, 优先级高于会话文件.
	EnvCookie = "CNKI_COOKIE"
	// EnvChromePath 用于指定 Chrome 可执行文件位置.
	EnvChromePath = "CNKI_CHROME_PATH"
	// EnvLogLevel 用于覆盖日志级别.
	EnvLogLevel = "CNKI_LOG_LEVEL"
	// EnvBaseURL 用于覆盖知网主站地址, 指向 mock 服务调试时使用.
	EnvBaseURL = "CNKI_BASE_URL"
)

// Dir 返回配置目录的绝对路径.
func Dir() string {
	if custom := os.Getenv(EnvConfigDir); custom != "" {
		return expand(custom)
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(expand(xdg), appDirName)
	}
	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(expand(appData), appDirName)
		}
	}
	return filepath.Join(homeDir(), ".config", appDirName)
}

// CacheDir 返回缓存目录的绝对路径.
func CacheDir() string {
	if custom := os.Getenv(EnvCacheDir); custom != "" {
		return expand(custom)
	}
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		return filepath.Join(expand(xdg), appDirName)
	}
	if runtime.GOOS == "windows" {
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			return filepath.Join(expand(local), appDirName)
		}
	}
	return filepath.Join(homeDir(), ".cache", appDirName)
}

// ConfigPath 返回配置文件的绝对路径.
func ConfigPath() string { return filepath.Join(Dir(), "config.toml") }

// SessionPath 返回会话文件的绝对路径.
//
// 会话里含知网 cookie, 属于凭证, 因此与配置同放在用户私有目录下.
func SessionPath() string { return filepath.Join(Dir(), "session.json") }

// ChromeProfileDir 返回受控浏览器使用的独立 profile 目录.
//
// 刻意与用户日常浏览器 profile 隔离, 避免污染其书签, 扩展与登录态.
func ChromeProfileDir() string { return filepath.Join(CacheDir(), "chrome-profile") }

func homeDir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return home
	}
	return "."
}

// expand 展开路径开头的波浪号.
func expand(path string) string {
	if path == "~" {
		return homeDir()
	}
	if len(path) > 2 && path[0] == '~' && (path[1] == '/' || path[1] == '\\') {
		return filepath.Join(homeDir(), path[2:])
	}
	return path
}
