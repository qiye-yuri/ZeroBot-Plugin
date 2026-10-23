package minecraftobserver

import (
	"encoding/json"
	"fmt"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
	"time"
)

// The status baseline and its per-subscription notices commit together.
type pendingNotice struct {
	ID          uint `gorm:"primary_key"`
	SubscribeID int64
	ServerAddr  string
	TargetID    int64
	TargetType  int64
	Body        string `gorm:"type:text"`
}

func (d *db) queueStatusChange(status *serverStatus, msg message.Message) error {
	raw, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	d.statusLock.Lock()
	defer d.statusLock.Unlock()
	d.subscribeLock.Lock()
	defer d.subscribeLock.Unlock()
	tx := d.sdb.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer tx.Rollback()
	var subscriptions []serverSubscribe
	if err = tx.Where("server_addr = ?", status.ServerAddr).Find(&subscriptions).Error; err != nil {
		return err
	}
	status.LastUpdate = time.Now().Unix()
	if err = tx.Where(&serverStatus{ServerAddr: status.ServerAddr}).Assign(statusFields(status)).FirstOrCreate(status).Error; err != nil {
		return err
	}
	for _, sub := range subscriptions {
		n := pendingNotice{SubscribeID: sub.ID, ServerAddr: sub.ServerAddr, TargetID: sub.TargetID, TargetType: sub.TargetType, Body: string(raw)}
		if err = tx.Create(&n).Error; err != nil {
			return err
		}
	}
	return tx.Commit().Error
}

// Delivery is at least once: a lost receipt or crash after sending may cause a retry.
// Stop later notices for the same recipient after a failure to retain event order.
func (d *db) deliverPending(call func(string, zero.Params) zero.APIResponse, enabled func(pendingNotice) bool) error {
	var notices []pendingNotice
	if err := d.sdb.Order("id asc").Find(&notices).Error; err != nil {
		return err
	}
	blocked := map[string]bool{}
	var firstErr error
	for _, n := range notices {
		key := fmt.Sprintf("%d/%d", n.TargetType, n.TargetID)
		var count int
		if err := d.sdb.Model(&serverSubscribe{}).Where("id = ? AND server_addr = ? AND target_id = ? AND target_type = ?", n.SubscribeID, n.ServerAddr, n.TargetID, n.TargetType).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 || !enabled(n) {
			if err := d.sdb.Delete(&n).Error; err != nil {
				return err
			}
			continue
		}
		if blocked[key] {
			continue
		}
		var msg message.Message
		if err := json.Unmarshal([]byte(n.Body), &msg); err != nil {
			return err
		}
		action, field := "send_private_msg", "user_id"
		if n.TargetType == targetTypeGroup {
			action, field = "send_group_msg", "group_id"
		}
		response := call(action, zero.Params{field: n.TargetID, "message": msg})
		if response.Status != "ok" || response.RetCode != 0 || response.Data.Get("message_id").Int() == 0 {
			blocked[key] = true
			if firstErr == nil {
				firstErr = fmt.Errorf("delivery failed for target type %d; queued for next pull", n.TargetType)
			}
			continue
		}
		if err := d.sdb.Delete(&n).Error; err != nil {
			return err
		}
	}
	return firstErr
}

// Maps preserve zero latency and clearing a previously non-empty status field.
func statusFields(s *serverStatus) map[string]interface{} {
	return map[string]interface{}{"server_addr": s.ServerAddr, "description": s.Description, "players": s.Players, "version": s.Version, "favicon_md5": s.FaviconMD5, "favicon_raw": s.FaviconRaw, "ping_delay": s.PingDelay, "last_update": s.LastUpdate}
}
