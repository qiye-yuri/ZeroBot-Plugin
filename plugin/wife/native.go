package wife

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

// Preserve the former group upload permission and images without migration.
var nativeEngine, nativeSettings = engine.Component("nwife", nil)

const nativeBase = "data/nwife"

func nativeFolder(gid int64) string {
	return filepath.Join(nativeBase, strconv.FormatInt(gid, 36))
}

func nativeNames(folder string) ([]string, error) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.Type().IsRegular() && !strings.HasPrefix(e.Name(), ".upload-") {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

func validNativeName(name string) bool {
	if name == "" || name == "." || name == ".." || len([]rune(name)) > 80 || strings.ContainsAny(name, `/\:*?"<>|`) || strings.HasSuffix(name, ".") {
		return false
	}
	for _, c := range name {
		if c < 32 {
			return false
		}
	}
	stem := strings.ToUpper(strings.Split(name, ".")[0])
	if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" {
		return false
	}
	if len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '1' && stem[3] <= '9' {
		return false
	}
	return true
}

func drawNative(ctx *zero.Ctx, emptyReply bool) bool {
	names, err := nativeNames(nativeFolder(ctx.Event.GroupID))
	if err != nil && !os.IsNotExist(err) {
		ctx.SendChain(message.Text("自定义角色库读取失败：", err))
		return true
	}
	if len(names) == 0 {
		if emptyReply {
			ctx.SendChain(message.Text("本群尚无自定义角色，请由管理员发送 添加wife名字 并附图片。"))
		}
		return emptyReply
	}
	if _, ok := ctx.State["args"].(string); !ok {
		ctx.State["args"] = ""
	}
	name := ctx.NickName()
	now := time.Now()
	sum := md5.Sum([]byte(fmt.Sprintf("%s%d%d%d", name, now.Year(), now.Month(), now.Day())))
	r := rand.New(rand.NewSource(int64(binary.LittleEndian.Uint64(sum[:]))))
	chosen := names[r.Intn(len(names))]
	path, err := filepath.Abs(filepath.Join(nativeFolder(ctx.Event.GroupID), chosen))
	if err != nil {
		ctx.SendChain(message.Text("自定义图片路径错误：", err))
		return true
	}
	ctx.SendChain(message.Text(name, "的今日角色是：", chosen), message.Image("file:///"+filepath.ToSlash(path)))
	return true
}

func init() {
	nativeEngine.OnPrefix("抽wife", zero.OnlyGroup).SetBlock(true).Handle(func(ctx *zero.Ctx) { drawNative(ctx, true) })
	nativeEngine.OnPrefix("添加wife", zero.OnlyGroup, func(ctx *zero.Ctx) bool {
		return nativeSettings.GetData(ctx.Event.GroupID)&1 != 0 || zero.AdminPermission(ctx)
	}, zero.MustProvidePicture).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		name := strings.ReplaceAll(strings.TrimPrefix(ctx.ExtractPlainText(), "添加wife"), " ", "")
		if !validNativeName(name) {
			ctx.SendChain(message.Text("角色名字无效，请使用不含路径符号的普通名字。"))
			return
		}
		folder := nativeFolder(ctx.Event.GroupID)
		if err := os.MkdirAll(folder, 0755); err != nil {
			ctx.SendChain(message.Text("保存失败：", err))
			return
		}
		tmp, err := os.CreateTemp(folder, ".upload-*")
		if err != nil {
			ctx.SendChain(message.Text("保存失败：", err))
			return
		}
		tmp.Close()
		defer os.Remove(tmp.Name())
		err = downloadNative(ctx.State["image_url"].([]string)[0], tmp.Name())
		if err == nil {
			err = os.Rename(tmp.Name(), filepath.Join(folder, name))
		}
		if err != nil {
			ctx.SendChain(message.Text("保存失败：", err))
			return
		}
		ctx.SendChain(message.Text("自定义角色已保存：", name))
	})
	nativeEngine.OnPrefix("删除wife", zero.OnlyGroup, zero.AdminPermission).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		name := strings.ReplaceAll(strings.TrimPrefix(ctx.ExtractPlainText(), "删除wife"), " ", "")
		if !validNativeName(name) {
			ctx.SendChain(message.Text("角色名字无效。"))
			return
		}
		if err := os.Remove(filepath.Join(nativeFolder(ctx.Event.GroupID), name)); err != nil {
			ctx.SendChain(message.Text("删除失败：", err))
			return
		}
		ctx.SendChain(message.Text("自定义角色已删除：", name))
	})
	nativeEngine.OnFullMatchGroup([]string{"让所有人均可添加wife", "不让所有人均可添加wife", "设置所有人均可添加wife", "授予所有人均可添加wife", "取消所有人均可添加wife", "撤销所有人均可添加wife"}, zero.SuperUserPermission, zero.OnlyGroup).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		text := ctx.ExtractPlainText()
		value := nativeSettings.GetData(ctx.Event.GroupID) &^ 1
		if strings.HasPrefix(text, "让") || strings.HasPrefix(text, "设置") || strings.HasPrefix(text, "授予") {
			value |= 1
		}
		if err := nativeSettings.SetData(ctx.Event.GroupID, value); err != nil {
			ctx.SendChain(message.Text("保存失败：", err))
			return
		}
		ctx.SendChain(message.Text("自定义角色上传权限已保存。"))
	})
}

func downloadNative(url, path string) error {
	client := &http.Client{Timeout: 20 * time.Second}
	rsp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return fmt.Errorf("图片下载状态码%d", rsp.StatusCode)
	}
	const limit = 10 << 20
	data, err := io.ReadAll(io.LimitReader(rsp.Body, limit+1))
	if err != nil {
		return err
	}
	if len(data) > limit {
		return fmt.Errorf("图片超过10MiB")
	}
	if _, _, err = image.DecodeConfig(bytes.NewReader(data)); err != nil {
		return fmt.Errorf("不是支持的图片：%w", err)
	}
	return os.WriteFile(path, data, 0644)
}
