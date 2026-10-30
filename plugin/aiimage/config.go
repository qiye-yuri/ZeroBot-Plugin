// Package aiimage 提供AI画图功能配置
package aiimage

import (
	"fmt"
	"strings"
	"sync"

	sql "github.com/FloatTech/sqlite"
)

// storage 管理画图配置存储
type storage struct {
	sync.RWMutex
	db   sql.Sqlite
	path string
}

// imageConfig 存储AI画图配置信息
type imageConfig struct {
	ID        int64  `db:"id"`        // 主键ID
	APIKey    string `db:"apiKey"`    // API密钥
	APIURL    string `db:"apiUrl"`    // API地址
	ModelName string `db:"modelName"` // 画图模型名称
}

// Separate table preserves the existing config schema and credentials.
type protocolConfig struct {
	ID   int64  `db:"id"`
	Mode string `db:"mode"`
}

func (sdb *storage) getProtocol() string {
	sdb.RLock()
	defer sdb.RUnlock()
	var cfg protocolConfig
	_ = sdb.db.Find("protocol", &cfg, "WHERE id = 1")
	if cfg.Mode == "" {
		return "siliconflow"
	}
	return cfg.Mode
}

func (sdb *storage) setProtocol(mode string) error {
	if mode != "openai" && mode != "siliconflow" {
		return fmt.Errorf("接口类型只能是 openai 或 siliconflow")
	}
	sdb.Lock()
	defer sdb.Unlock()
	if sdb.path != "" {
		var old protocolConfig
		_ = sdb.db.Find("protocol", &old, "WHERE id = 1")
		if old.Mode != mode {
			if _, err := sdb.db.Exec("UPDATE render_settings SET customCost = ? WHERE id = 1", ""); err != nil {
				return err
			}
		}
	}
	return sdb.db.Insert("protocol", &protocolConfig{ID: 1, Mode: mode})
}

// getConfig 获取当前配置
func (sdb *storage) getConfig() imageConfig {
	sdb.RLock()
	defer sdb.RUnlock()
	cfg := imageConfig{}
	_ = sdb.db.Find("config", &cfg, "WHERE id = 1")
	return cfg
}

// setConfig 设置AI画图配置
func (sdb *storage) setConfig(apiKey, apiURL, modelName string) error {
	sdb.Lock()
	defer sdb.Unlock()
	return sdb.db.Insert("config", &imageConfig{
		ID:        1,
		APIKey:    apiKey,
		APIURL:    apiURL,
		ModelName: modelName,
	})
}

// PrintConfig 返回格式化后的配置信息
func (sdb *storage) PrintConfig() string {
	cfg := sdb.getConfig()
	var builder strings.Builder
	builder.WriteString("当前AI画图配置:\n")
	keyStatus := "未设置（无鉴权本地服务可留空）"
	if cfg.APIKey != "" {
		keyStatus = "已设置（隐藏）"
	}
	builder.WriteString(fmt.Sprintf("• 密钥: %s\n", keyStatus))
	builder.WriteString(fmt.Sprintf("• 接口类型: %s\n", sdb.getProtocol()))
	builder.WriteString(fmt.Sprintf("• 接口地址: %s\n", cfg.APIURL))
	builder.WriteString(fmt.Sprintf("• 模型名: %s\n", cfg.ModelName))
	return builder.String()
}
