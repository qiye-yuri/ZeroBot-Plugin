// Explicit live test: downloads a pack and sends it only to the supplied group.
package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/FloatTech/ZeroBot-Plugin/internal/ctb"
	"github.com/RomiChan/websocket"
)

type reply struct {
	Status  string          `json:"status"`
	Retcode int             `json:"retcode"`
	Echo    string          `json:"echo"`
	Data    json.RawMessage `json:"data"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	group := flag.Int64("group", 0, "Explicit authorized test group")
	send := flag.Bool("send", false, "Authorize real group messages and a file upload")
	pack := flag.String("pack", "135", "Pack ID")
	version := flag.String("version", "100577", "Version ID")
	mode := flag.String("mode", "full", "light/full")
	existing := flag.String("file", "", "Resend an already downloaded ZIP without another download")
	flag.Parse()
	if !*send || *group <= 0 {
		return fmt.Errorf("requires -send and an explicitly authorized -group")
	}
	snap, err := ctb.Default.Settings("", nil)
	if err != nil {
		return err
	}
	if !snap.Config.Enabled || !snap.Config.Upload || snap.Config.GroupDelivery != "group" {
		return fmt.Errorf("enable tool and file sending with group delivery first")
	}
	var config struct {
		Zero struct {
			Owners []int64 `json:"super_users"`
		} `json:"zero"`
		WS []struct {
			URL   string `json:"Url"`
			Token string `json:"AccessToken"`
		} `json:"ws"`
	}
	b, err := os.ReadFile(".local/config.json")
	if err != nil {
		return err
	}
	if err = json.Unmarshal(b, &config); err != nil {
		return err
	}
	var endpoint, token string
	for _, v := range config.WS {
		if v.URL == "ws://127.0.0.1:6700" {
			endpoint, token = v.URL, v.Token
		}
	}
	if endpoint == "" || len(config.Zero.Owners) == 0 {
		return fmt.Errorf("local OneBot/owner unavailable")
	}
	conn, _, err := websocket.DefaultDialer.Dial(endpoint, http.Header{"Authorization": []string{"Bearer " + token}, "X-Client-Role": []string{"Universal"}})
	if err != nil {
		return fmt.Errorf("OneBot connection failed")
	}
	defer conn.Close()
	seq := 0
	call := func(action string, p map[string]any) (reply, error) {
		seq++
		echo := fmt.Sprintf("ctb-live-%d-%d", time.Now().UnixNano(), seq)
		conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
		conn.SetReadDeadline(time.Now().Add(10 * time.Minute))
		if e := conn.WriteJSON(map[string]any{"action": action, "params": p, "echo": echo}); e != nil {
			return reply{}, e
		}
		for {
			var v reply
			var raw json.RawMessage
			if e := conn.ReadJSON(&raw); e != nil {
				return v, e
			}
			var envelope struct {
				Echo string `json:"echo"`
			}
			if e := json.Unmarshal(raw, &envelope); e != nil {
				return v, e
			}
			if envelope.Echo != echo {
				continue
			}
			if e := json.Unmarshal(raw, &v); e != nil {
				return v, e
			}
			if v.Status != "ok" || v.Retcode != 0 {
				return v, fmt.Errorf("%s failed: %s/%d", action, v.Status, v.Retcode)
			}
			return v, nil
		}
	}
	account, err := call("get_login_info", map[string]any{})
	if err != nil {
		return err
	}
	var login struct {
		UserID int64 `json:"user_id"`
	}
	json.Unmarshal(account.Data, &login)
	var expected struct {
		QQ string `json:"expectedQQ"`
	}
	b, err = os.ReadFile(".local/onebot/account.json")
	if err != nil {
		return err
	}
	if err = json.Unmarshal(b, &expected); err != nil {
		return err
	}
	if fmt.Sprint(login.UserID) != expected.QQ {
		return fmt.Errorf("bot identity mismatch")
	}
	if _, err = call("get_group_info", map[string]any{"group_id": *group}); err != nil {
		return err
	}
	notify := func(text string) error {
		_, e := call("send_group_msg", map[string]any{"group_id": *group, "message": text})
		return e
	}
	r := ctb.Request{Action: "download", Pack: *pack, Version: *version, Mode: *mode}
	var result ctb.Result
	if *existing == "" {
		if err = notify("FTB下载与群文件发送测试开始：" + *pack + " / " + *version + " / " + *mode + "。本次会占用本群1次下载额度。下一步：等待下载和ZIP校验完成，请勿重复提交。"); err != nil {
			return err
		}
		result, err = ctb.Default.RunFor(context.Background(), r, *group)
		if err != nil {
			notify("FTB下载测试失败。下一步：请等待主人排查，不必重复提交。")
			return err
		}
	} else {
		file, e := filepath.Abs(*existing)
		if e != nil {
			return e
		}
		z, e := zip.OpenReader(file)
		if e != nil {
			return e
		}
		for _, f := range z.File {
			rd, e := f.Open()
			if e != nil {
				z.Close()
				return e
			}
			_, e = io.Copy(io.Discard, rd)
			rd.Close()
			if e != nil {
				z.Close()
				return e
			}
		}
		z.Close()
		result.Files = []string{file}
		if err = notify("继续发送已下载并校验的测试文件，不重新下载、不增加次数。下一步：等待群文件消息。"); err != nil {
			return err
		}
	}
	if err = notify("FTB下载与ZIP校验完成，正在发送群文件。下一步：等待文件消息出现。"); err != nil {
		return err
	}
	var fileID string
	err = ctb.Deliver(result.Files, config.Zero.Owners[0], *group, "group", func(action string, p map[string]any) error {
		v, e := call(action, p)
		if e == nil {
			var data struct {
				ID string `json:"file_id"`
			}
			json.Unmarshal(v.Data, &data)
			fileID = data.ID
		}
		return e
	})
	if err != nil {
		notify("群文件发送接口未确认成功，文件已保留本机。下一步：请主人检查群文件权限与空间，不必重新下载。")
		return err
	}
	listed := false
	var listingErr string
	for i := 0; i < 3; i++ {
		v, e := call("get_group_root_files", map[string]any{"group_id": *group, "file_count": 100})
		if e != nil {
			listingErr = e.Error()
			break
		}
		var data struct {
			Files []struct {
				ID   string `json:"file_id"`
				Name string `json:"file_name"`
				Size int64  `json:"file_size"`
			} `json:"files"`
		}
		if e := json.Unmarshal(v.Data, &data); e != nil {
			listingErr = "群文件列表格式解析失败"
			break
		}
		st, e := os.Stat(result.Files[0])
		if e != nil {
			return e
		}
		for _, f := range data.Files {
			// NapCat's message file UUID and group-list file ID use different
			// namespaces. The list check verifies name and size, not ID equality.
			if f.Name == filepath.Base(result.Files[0]) && f.Size == st.Size() {
				listed = true
			}
		}
		if listed {
			break
		}
		time.Sleep(2 * time.Second)
	}
	if err = notify("QQ已接受测试文件发送。下一步：打开本群聊天或群文件，找到 " + filepath.Base(result.Files[0]) + " 并下载；请确认是否能收到。保存ZIP后再导入兼容启动器，暂未验证游戏启动。以后群内@机器人发送FTB搜索 Evolution，按返回提示继续。"); err != nil {
		return err
	}
	st, _ := os.Stat(result.Files[0])
	evidence := map[string]any{"group": *group, "file": result.Files[0], "bytes": st.Size(), "fileID": fileID, "uploadAccepted": true, "verifiedInGroupFiles": listed, "listingError": listingErr, "recipientConfirmed": false, "completedAt": time.Now().Format(time.RFC3339)}
	b, _ = json.MarshalIndent(evidence, "", "  ")
	os.MkdirAll(".local/ctb-delivery", 0700)
	os.WriteFile(".local/ctb-delivery/latest.json", b, 0600)
	fmt.Println(string(b))
	return nil
}
