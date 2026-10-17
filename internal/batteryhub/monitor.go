// Package batteryhub provides private, deduplicated peripheral battery alerts.
package batteryhub

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	log "github.com/sirupsen/logrus"
	zero "github.com/wdvxdr1123/ZeroBot"
)

type Reading struct {
	Name    string
	Percent *int
}

// Next keeps both thresholds latched until the device recovers to >= 10%.
// Missing readings never reset a latch. A jump straight to <= 5% sends one alert.
func Next(previous int, percent *int) (state, alert int) {
	if percent == nil || *percent < 0 || *percent > 100 {
		return previous, 0
	}
	if *percent >= 10 {
		return 0, 0
	}
	level := 1
	if *percent <= 5 {
		level = 2
	}
	if level > previous {
		return level, level
	}
	return previous, 0
}

type monitor struct {
	state map[string]int
	path  string
	send  func(string) bool
}

func (m *monitor) update(readings []Reading) error {
	for _, r := range readings {
		next, alert := Next(m.state[r.Name], r.Percent)
		if alert != 0 {
			label := "低于 10%"
			if alert == 2 {
				label = "已降至 5% 或以下"
			}
			if !m.send(fmt.Sprintf("【设备低电量警告】%s 剩余 %d%%，%s。请及时充电或更换电池。", r.Name, *r.Percent, label)) {
				continue
			}
		}
		if next == m.state[r.Name] {
			continue
		}
		m.state[r.Name] = next
		b, err := json.Marshal(m.state)
		if err != nil {
			return err
		}
		if err = os.WriteFile(m.path+".tmp", b, 0600); err != nil {
			return err
		}
		if err = os.Rename(m.path+".tmp", m.path); err != nil {
			return err
		}
	}
	return nil
}

// Start enables the monitor only when its locally installed helper exists and
// exactly one owner is configured, so alerts cannot fan out to other people.
func Start(owners []int64) {
	if runtime.GOOS != "windows" {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	helper := filepath.Join(filepath.Dir(exe), "battery-hub", "BatteryProbe.exe")
	if _, err = os.Stat(helper); err != nil {
		return
	}
	if len(owners) != 1 || owners[0] <= 0 {
		log.Warn("[battery-hub] 需要唯一主人账号，未启动电量提醒")
		return
	}
	owner := owners[0]
	dir := filepath.Join(".local", "battery-hub")
	if err = os.MkdirAll(dir, 0700); err != nil {
		log.Error("[battery-hub] ", err)
		return
	}
	m := &monitor{state: map[string]int{}, path: filepath.Join(dir, fmt.Sprintf("state-%d.json", owner))}
	b, err := os.ReadFile(m.path)
	if err == nil {
		err = json.Unmarshal(b, &m.state)
		if m.state == nil && err == nil {
			err = fmt.Errorf("invalid battery state")
		}
	}
	if err != nil && !os.IsNotExist(err) {
		log.Error("[battery-hub] 状态不可读取，暂停提醒: ", err)
		return
	}
	m.send = func(text string) bool {
		sent := false
		zero.RangeBot(func(_ int64, ctx *zero.Ctx) bool {
			sent = ctx.SendPrivateMessage(owner, text) != 0
			return false
		})
		return sent
	}
	go func() {
		log.Info("[battery-hub] 电量私聊提醒已启动：每60秒检查，低于10%及5%或以下")
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			command := exec.CommandContext(ctx, helper)
			hideWindow(command)
			out, err := command.Output()
			cancel()
			if err != nil {
				log.Warn("[battery-hub] 电量查询失败")
			} else {
				var readings []Reading
				if err = json.Unmarshal(out, &readings); err != nil {
					log.Warn("[battery-hub] 电量结果无效")
				} else if err = m.update(readings); err != nil {
					log.Error("[battery-hub] 保存状态失败，暂停提醒: ", err)
					return
				}
			}
			<-ticker.C
		}
	}()
}
