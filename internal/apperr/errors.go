// Package apperr 定义全项目统一的错误类型与退出码.
//
// 退出码约定: 0 成功, 1 通用失败, 2 用法错误, 3 会话失效,
// 4 权限不足, 5 远端接口异常, 130 被用户中断.
package apperr

import (
	"errors"
	"fmt"
)

// ExitCode 是进程退出码.
type ExitCode int

const (
	ExitOK         ExitCode = 0
	ExitFailure    ExitCode = 1
	ExitUsage      ExitCode = 2
	ExitSession    ExitCode = 3
	ExitPermission ExitCode = 4
	ExitRemote     ExitCode = 5
	ExitInterrupt  ExitCode = 130
)

// Error 是携带退出码的错误.
type Error struct {
	Code ExitCode
	Msg  string
	Err  error
}

// Error 实现 error 接口.
func (e *Error) Error() string {
	if e.Err == nil {
		return e.Msg
	}
	if e.Msg == "" {
		return e.Err.Error()
	}
	return e.Msg + ": " + e.Err.Error()
}

// Unwrap 支持 errors.Is / errors.As 向上匹配.
func (e *Error) Unwrap() error { return e.Err }

// New 构造一个只带消息的错误.
func New(code ExitCode, format string, args ...any) *Error {
	return &Error{Code: code, Msg: fmt.Sprintf(format, args...)}
}

// Wrap 在已有错误上附加消息与退出码.
func Wrap(code ExitCode, err error, format string, args ...any) *Error {
	return &Error{Code: code, Msg: fmt.Sprintf(format, args...), Err: err}
}

// Usage 构造用法错误.
func Usage(format string, args ...any) *Error { return New(ExitUsage, format, args...) }

// Session 构造会话失效错误, 由调用方在捕获到验证跳转时抛出.
func Session(format string, args ...any) *Error { return New(ExitSession, format, args...) }

// Permission 构造权限不足错误.
func Permission(format string, args ...any) *Error { return New(ExitPermission, format, args...) }

// Remote 构造远端异常错误.
func Remote(format string, args ...any) *Error { return New(ExitRemote, format, args...) }

// ExitCodeOf 抽取错误携带的退出码, 未标记的错误统一按通用失败处理.
func ExitCodeOf(err error) ExitCode {
	if err == nil {
		return ExitOK
	}
	var target *Error
	if errors.As(err, &target) {
		return target.Code
	}
	return ExitFailure
}
