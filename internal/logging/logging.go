// Package logging 提供全项目统一的 slog 日志设施.
//
// 关键约束: 日志只写 stderr. stdout 留给结构化结果与人类可读表格,
// 任何日志行混进 stdout 都会破坏管道用法.
package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

// Options 描述日志初始化参数.
type Options struct {
	// Level 是日志级别文本, 取值为 trace / debug / info / warn / error.
	Level string
	// Writer 是日志输出目标, 为空时使用 stderr.
	Writer io.Writer
	// Format 为 text 或 json, 默认 text.
	Format string
}

// New 构造一个日志器.
func New(opts Options) *slog.Logger {
	writer := opts.Writer
	if writer == nil {
		writer = os.Stderr
	}
	handlerOpts := &slog.HandlerOptions{Level: ParseLevel(opts.Level)}

	var handler slog.Handler
	if strings.EqualFold(opts.Format, "json") {
		handler = slog.NewJSONHandler(writer, handlerOpts)
	} else {
		handler = slog.NewTextHandler(writer, handlerOpts)
	}
	return slog.New(handler)
}

// ParseLevel 把级别文本解析为 slog 级别, 无法识别时回落到 info.
func ParseLevel(text string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "trace", "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Discard 返回一个丢弃全部日志的日志器, 供测试使用.
func Discard() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError + 1}))
}

// Redact 把敏感字符串裁剪为可安全打印的预览形式.
//
// cookie 与会话标识只在日志里以首尾各四个字符的形式出现, 中间部分永不落盘.
func Redact(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 12 {
		return "***"
	}
	return value[:4] + "..." + value[len(value)-4:]
}

// Keys 返回 map 的键名列表, 用于在 debug 日志里描述请求形状而不泄漏取值.
func Keys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

// FromContext 取出上下文中的日志器, 不存在时返回默认日志器.
func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok && logger != nil {
		return logger
	}
	return slog.Default()
}

// WithContext 把日志器放入上下文.
func WithContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

type loggerKey struct{}
