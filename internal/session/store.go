package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/azazo1/cnki-cli/internal/config"
)

// CurrentSchemaVersion 是当前代码期望的会话文件版本号.
//
// 会话结构发生不兼容调整时必须递增, 并在 migrate.go 补上对应升级步骤.
const CurrentSchemaVersion = 1

// Load 读取磁盘上的会话. 文件不存在时返回 nil, nil.
func Load() (*Session, error) {
	path := config.SessionPath()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取会话 %s: %w", path, err)
	}

	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("解析会话 %s: %w", path, err)
	}
	if err := Migrate(&s); err != nil {
		return nil, fmt.Errorf("迁移会话 %s: %w", path, err)
	}
	if s.Account == (Account{}) {
		s.Account = ParseAccount(&s)
	}
	return &s, nil
}

// Save 把会话写入磁盘.
//
// 文件权限固定为 0600: 会话等于账号访问权, 不能给同机其他用户可读的机会.
func Save(s *Session) error {
	if s == nil {
		return errors.New("会话为空")
	}
	path := config.SessionPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("创建配置目录: %w", err)
	}
	s.Cookies = Normalize(s.Cookies)
	if s.Account == (Account{}) {
		s.Account = ParseAccount(s)
	}
	if s.SchemaVersion == 0 {
		s.SchemaVersion = CurrentSchemaVersion
	}
	s.UpdatedAt = nowFunc()

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化会话: %w", err)
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// Clear 删除磁盘上的会话文件.
func Clear() error {
	path := config.SessionPath()
	err := os.Remove(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
