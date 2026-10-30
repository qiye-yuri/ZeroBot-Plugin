package aiimage

import (
	"crypto/sha256"
	dbsql "database/sql"
	"encoding/json"
	"errors"
	"fmt"
	core "github.com/FloatTech/ZeroBot-Plugin/internal/imagegen"
	sql "github.com/FloatTech/sqlite"
	"net/url"
	"strings"
	"time"
)

func (s *storage) open(path string) error {
	s.Lock()
	defer s.Unlock()
	if s.path != "" {
		return nil
	}
	s.db = sql.New(path)
	if err := s.db.Open(time.Hour); err != nil {
		return err
	}
	if err := s.db.Create("config", &imageConfig{}); err != nil {
		return err
	}
	if err := s.db.Create("protocol", &protocolConfig{}); err != nil {
		return err
	}
	if err := s.db.Create("render_settings", &renderSettings{}); err != nil {
		return err
	}
	if _, err := s.db.Exec("ALTER TABLE render_settings ADD COLUMN options TEXT NOT NULL DEFAULT '{}'"); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		return err
	}
	s.path = path
	return nil
}
func (s *storage) readLocked() (imageConfig, string, error) {
	var c imageConfig
	if err := s.db.Find("config", &c, "WHERE id = 1"); err != nil && !errors.Is(err, sql.ErrNullResult) {
		return c, "", err
	}
	var p protocolConfig
	if err := s.db.Find("protocol", &p, "WHERE id = 1"); err != nil && !errors.Is(err, sql.ErrNullResult) {
		return c, "", err
	}
	if p.Mode == "" {
		p.Mode = "siliconflow"
	}
	return c, p.Mode, nil
}
func revision(c imageConfig, mode string, settings ...renderSettings) string {
	b, _ := json.Marshal([]any{c, mode, settings})
	return fmt.Sprintf("%x", sha256.Sum256(b))
}
func (s *storage) admin(next *core.Update, backup core.Backup) (out core.Snapshot, err error) {
	s.Lock()
	defer s.Unlock()
	c, mode, err := s.readLocked()
	if err != nil {
		return out, fmt.Errorf("画图配置读取失败")
	}
	settings, err := s.settingsLocked()
	if err != nil {
		return out, fmt.Errorf("画图预算配置读取失败")
	}
	if next != nil {
		if next.Revision != revision(c, mode, settings) {
			return out, fmt.Errorf("配置已被其他页面或私聊修改，请重新读取后保存")
		}
		if next.Mode != "openai" && next.Mode != "siliconflow" {
			return out, fmt.Errorf("请选择有效接口类型")
		}
		address := strings.TrimSpace(next.URL)
		model := strings.TrimSpace(next.Model)
		u, e := url.Parse(address)
		if e != nil || !validImageURL(address) || u.RawQuery != "" || u.Fragment != "" || len(address) > 2048 {
			return out, fmt.Errorf("请输入完整HTTP/HTTPS生成接口地址，不包含凭据、查询参数或片段")
		}
		if model == "" || len(model) > 256 {
			return out, fmt.Errorf("模型名称不能为空且不能超过256字节")
		}
		updatedSettings := settings
		if next.Quality != "" {
			updatedSettings.Quality = next.Quality
		}
		if next.Size != "" {
			updatedSettings.Size = normalizeSize(next.Size)
		}
		if !validQuality(updatedSettings.Quality) || !validSize(updatedSettings.Size) {
			return out, fmt.Errorf("请选择支持的质量与合法尺寸")
		}
		if next.MonthlyBudgetUSD != "" {
			amount, e := parseDollars(next.MonthlyBudgetUSD, 100000*usd)
			if e != nil {
				return out, e
			}
			updatedSettings.Budget = amount
		}
		updatedSettings.CustomCost = strings.TrimSpace(next.CustomCostUSD)
		if updatedSettings.CustomCost != "" {
			if _, e := parseDollars(updatedSettings.CustomCost, 10000*usd); e != nil {
				return out, e
			}
		}
		if officialEndpoint(address) && updatedSettings.CustomCost != "" {
			return out, fmt.Errorf("OpenAI官方接口使用已核实价格，不接受自定义费用")
		}
		if next.Options != nil {
			options := *next.Options
			if options.ReferenceReserveUSD == "" {
				previous, e := optionsFor(settings)
				if e != nil {
					return out, e
				}
				options.ReferenceReserveUSD = previous.ReferenceReserveUSD
			}
			putOptions(&updatedSettings, options)
		}
		updated := c
		updated.ID = 1
		updated.APIURL = address
		updated.ModelName = model
		if err := validateOptions(updated, next.Mode, updatedSettings); err != nil {
			return out, err
		}
		switch next.Key.Action {
		case "keep":
			if next.Key.Value != "" || next.Key.Confirm {
				return out, fmt.Errorf("保留密钥无需输入或确认")
			}
		case "replace":
			if strings.TrimSpace(next.Key.Value) == "" || len(next.Key.Value) > 4096 || strings.ContainsAny(next.Key.Value, "\r\n") || next.Key.Confirm {
				return out, fmt.Errorf("请填写有效新密钥")
			}
			updated.APIKey = strings.TrimSpace(next.Key.Value)
		case "clear":
			if !next.Key.Confirm || next.Key.Value != "" {
				return out, fmt.Errorf("请确认清除密钥")
			}
			updated.APIKey = ""
		default:
			return out, fmt.Errorf("请选择密钥操作")
		}
		if revision(updated, next.Mode, updatedSettings) != revision(c, mode, settings) {
			if backup == nil {
				return out, fmt.Errorf("画图备份不可用，未保存")
			}
			if e = backup(&s.db); e != nil {
				return out, fmt.Errorf("画图配置备份失败，未保存")
			}
			database, e := dbsql.Open(sql.DriverName, s.path)
			if e != nil {
				return out, fmt.Errorf("画图配置打开失败")
			}
			defer database.Close()
			tx, e := database.Begin()
			if e != nil {
				return out, fmt.Errorf("画图配置事务创建失败")
			}
			defer tx.Rollback()
			_, e = tx.Exec("INSERT OR REPLACE INTO config (id,apiKey,apiUrl,modelName) VALUES (1,?,?,?)", updated.APIKey, updated.APIURL, updated.ModelName)
			if e == nil {
				_, e = tx.Exec("INSERT OR REPLACE INTO protocol (id,mode) VALUES (1,?)", next.Mode)
			}
			if e == nil {
				_, e = tx.Exec("INSERT OR REPLACE INTO render_settings (id,quality,size,budget,customCost,options) VALUES (1,?,?,?,?,?)", updatedSettings.Quality, updatedSettings.Size, updatedSettings.Budget, updatedSettings.CustomCost, updatedSettings.OptionsJSON)
				if e == nil {
					e = tx.Commit()
				}
			}
			if e != nil {
				return out, fmt.Errorf("画图配置保存失败，原设置保留")
			}
			c = updated
			mode = next.Mode
			settings = updatedSettings
		}
	}
	out = core.Snapshot{Revision: revision(c, mode, settings), Mode: mode, URL: c.APIURL, Model: c.ModelName, KeyConfigured: c.APIKey != ""}
	out.Options, err = optionsFor(settings)
	if err != nil {
		return out, err
	}
	out.Quality = settings.Quality
	out.Size = settings.Size
	out.MonthlyBudgetUSD = dollars(settings.Budget)
	out.CustomCostUSD = settings.CustomCost
	out.Budget, err = s.budgetLocked(time.Now(), settings.Budget)
	if err != nil {
		return core.Snapshot{}, fmt.Errorf("预算账本读取失败，画图已暂停")
	}
	_, note, quoteErr := quoteImage(c, mode, "", settings)
	out.PricingReady = quoteErr == nil
	out.PricingNote = note
	if quoteErr != nil {
		out.PricingNote = quoteErr.Error()
	}
	// Legacy URLs may contain credentials; never expose them in a browser response.
	if u, e := url.Parse(out.URL); e == nil {
		u.User = nil
		u.RawQuery = ""
		u.Fragment = ""
		out.URL = u.String()
	} else {
		out.URL = ""
	}
	return out, nil
}

// Update one chat field under the same lock used by browser saves.
func (s *storage) updateField(field, value string) error {
	s.Lock()
	defer s.Unlock()
	c, _, err := s.readLocked()
	if err != nil {
		return err
	}
	if (field == "模型名" && value != c.ModelName) || (field == "接口地址" && value != c.APIURL) {
		if _, err = s.db.Exec("UPDATE render_settings SET customCost = ? WHERE id = 1", ""); err != nil {
			return err
		}
	}
	switch field {
	case "密钥":
		c.APIKey = value
	case "接口地址":
		c.APIURL = value
	case "模型名":
		c.ModelName = value
	default:
		return fmt.Errorf("未知字段")
	}
	c.ID = 1
	return s.db.Insert("config", &c)
}
func (s *storage) generationConfig() (imageConfig, string, error) {
	s.RLock()
	defer s.RUnlock()
	return s.readLocked()
}
