package cnki

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/azazo1/cnki-cli/internal/apperr"
	"github.com/azazo1/cnki-cli/internal/config"
	"github.com/azazo1/cnki-cli/internal/taxonomy"
)

// 上次检索式文件的版本号, 结构不兼容时递增.
const lastQuerySchemaVersion = 1

// lastQueryFileName 是上次检索式的文件名.
const lastQueryFileName = "last-query.json"

// LastQueryPath 返回上次检索式的存放位置.
//
// 放在缓存目录而不是配置目录: 它随每次检索变化, 属于可丢弃的中间状态,
// 删掉只会让"结果中检索"失去接续点, 不影响其它功能.
func LastQueryPath() string {
	return filepath.Join(config.CacheDir(), lastQueryFileName)
}

// lastQueryRecord 是落盘的检索式快照.
type lastQueryRecord struct {
	SchemaVersion int       `json:"schema_version"`
	SavedAt       time.Time `json:"saved_at"`
	SearchType    string    `json:"search_type"`
	Description   string    `json:"description"`
	QueryJSON     QueryJSON `json:"query_json"`
}

// SaveLastQuery 记录本次检索式, 供"结果中检索"接续.
//
// 写盘失败不应当影响检索本身, 因此调用方只需记录警告.
func SaveLastQuery(q *Query) error {
	if q == nil {
		return errors.New("检索式为空")
	}
	path := LastQueryPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("创建缓存目录: %w", err)
	}

	record := lastQueryRecord{
		SchemaVersion: lastQuerySchemaVersion,
		SavedAt:       time.Now(),
		SearchType:    q.searchType.Name(),
		Description:   q.description,
		QueryJSON:     q.QueryJSON(),
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化检索式: %w", err)
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// LoadLastQuery 读回上次检索式.
//
// 没有任何记录时给出可直接照做的提示, 而不是让用户去猜为什么收窄失败.
func LoadLastQuery() (*Query, error) {
	path := LastQueryPath()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, apperr.Usage(
			"还没有可接续的检索式, 请先执行一次检索 (例如 cnki search 深度学习), 再用 refine 收窄")
	}
	if err != nil {
		return nil, apperr.Wrap(apperr.ExitFailure, err, "读取上次检索式 %s", path)
	}

	var record lastQueryRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, apperr.Wrap(apperr.ExitFailure, err, "解析上次检索式 %s", path)
	}
	if record.SchemaVersion > lastQuerySchemaVersion {
		return nil, apperr.New(apperr.ExitFailure,
			"上次检索式的版本 %d 高于本程序支持的 %d, 请升级 cnki-cli",
			record.SchemaVersion, lastQuerySchemaVersion)
	}
	return queryFromRecord(record)
}

// ClearLastQuery 删除上次检索式记录.
func ClearLastQuery() error {
	err := os.Remove(LastQueryPath())
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// queryFromRecord 把快照还原成可再次提交的检索式.
func queryFromRecord(record lastQueryRecord) (*Query, error) {
	payload := record.QueryJSON
	if payload.SearchType == 0 {
		return nil, apperr.New(apperr.ExitFailure, "上次检索式记录缺少检索方式, 无法接续")
	}
	if len(payload.QNode.QGroup) == 0 {
		return nil, apperr.New(apperr.ExitFailure, "上次检索式记录缺少检索条件, 无法接续")
	}

	return &Query{
		searchType:  taxonomy.SearchType(payload.SearchType),
		groups:      payload.QNode.QGroup,
		products:    payload.Products,
		description: record.Description,
		opts: SearchOptions{
			Classid:    payload.Classid,
			Resource:   payload.Resource,
			KuaKuCode:  payload.KuaKuCode,
			Rlang:      payload.Rlang,
			SearchFrom: taxonomy.SearchFrom(payload.SearchFrom),
			ExScope:    truthy(payload.ExScope),
			SimpTrad:   strings.TrimSpace(payload.SimpTrad) == "1",
		},
	}, nil
}

// truthy 判断知网的开关取值是否为真.
//
// 该字段在不同检索方式下分别是数字与字符串, 因此两种形态都要认.
func truthy(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case float64:
		return typed != 0
	case int:
		return typed != 0
	case string:
		trimmed := strings.TrimSpace(typed)
		return trimmed != "" && trimmed != "0" && !strings.EqualFold(trimmed, "false")
	default:
		return false
	}
}
