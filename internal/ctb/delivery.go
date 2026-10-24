package ctb

import (
	"fmt"
	"path/filepath"
)

// CanUse keeps private access owner-only and makes group access explicit.
func CanUse(group, mentioned, owner, admin bool, access, action string) bool {
	if !group {
		return owner
	}
	if !mentioned {
		return false
	}
	switch access {
	case "all":
		return true
	case "owner":
		return owner
	case "admin":
		return action != "download" || owner || admin
	default:
		return false
	}
}

// Deliver chooses the source group or requester from the saved delivery policy.
func Deliver(files []string, requester, group int64, delivery string, send func(string, map[string]any) error) error {
	if requester <= 0 || len(files) == 0 {
		return fmt.Errorf("缺少接收人或下载文件")
	}
	action, key, target := "upload_private_file", "user_id", requester
	if group > 0 && delivery == "group" {
		action, key, target = "upload_group_file", "group_id", group
	}
	for _, file := range files {
		if err := send(action, map[string]any{key: target, "file": file, "name": filepath.Base(file)}); err != nil {
			return err
		}
	}
	return nil
}

func NextStep(r Request) string {
	switch r.Action {
	case "search":
		return "下一步：从结果选择整合包ID，发送 FTB查询 ID，例如 FTB查询 135。"
	case "inspect":
		return fmt.Sprintf("下一步：发送 FTB下载 %s latest light（轻量包）或 FTB下载 %s latest full（标准客户端包）。指定旧版本时，用表格第一列数字ID替换latest，不是1.5.1这样的版本名称。", r.Pack, r.Pack)
	default:
		return "下一步：接收并保存ZIP，在支持该格式的启动器中导入整合包；轻量包导入后仍需下载模组。若导入失败，请把启动器名称和错误提示发给主人排查。"
	}
}
