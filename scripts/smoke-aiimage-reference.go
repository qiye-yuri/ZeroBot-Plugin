//go:build ignore

// Isolated OneBot -> exact reference pixels -> JSON edit endpoint -> image reply.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	qa "github.com/FloatTech/ZeroBot-Plugin/internal/adminqa"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

var candidateBinary = flag.String("binary", ".local/bin/zerobot-aiimage-reference.exe", "isolated candidate executable")

func main() {
	flag.Parse()
	absolute, err := filepath.Abs(*candidateBinary)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	*candidateBinary = absolute
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	parent := filepath.Join(root, ".local/aiimage-reference-acceptance")
	os.MkdirAll(parent, 0700)
	out, err := os.MkdirTemp(parent, "run-")
	if err != nil {
		return err
	}
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	var source bytes.Buffer
	if err = png.Encode(&source, img); err != nil {
		return err
	}
	expected := sha256.Sum256(source.Bytes())
	var gets, edits, generations, bad atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			gets.Add(1)
			if r.Header.Get("Authorization") != "" {
				bad.Add(1)
			}
			if r.URL.Path != "/source.png" {
				http.Error(w, "missing", 404)
				return
			}
			w.Write(source.Bytes())
			return
		}
		var body struct {
			Model, Prompt string
			Images        []struct {
				URL string `json:"image_url"`
			} `json:"images"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			bad.Add(1)
			http.Error(w, "bad", 400)
			return
		}
		if r.URL.Path == "/v1/images/edits" {
			edits.Add(1)
			if len(body.Images) != 1 {
				bad.Add(1)
			} else {
				_, b64, ok := strings.Cut(body.Images[0].URL, ",")
				data, e := base64.StdEncoding.DecodeString(b64)
				if !ok || e != nil || sha256.Sum256(data) != expected {
					bad.Add(1)
				}
			}
		} else if r.URL.Path == "/v1/images/generations" {
			generations.Add(1)
			if len(body.Images) != 0 {
				bad.Add(1)
			}
		} else {
			bad.Add(1)
		}
		if r.Header.Get("Authorization") != "Bearer synthetic-key" {
			bad.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"b64_json": base64.StdEncoding.EncodeToString(source.Bytes())}}})
	}))
	defer server.Close()
	h, err := qa.New(root, *candidateBinary, out)
	if err != nil {
		return err
	}
	defer h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	bot, err := h.Start(ctx, "reference")
	if err != nil {
		return err
	}
	defer bot.Stop()
	c := qa.NewClient(h.Address)
	if _, err = c.Auth(h.AdminDir); err != nil {
		return err
	}
	plugins, err := c.Checked("GET", "/api/plugins", nil)
	if err != nil {
		return err
	}
	for _, raw := range plugins["plugins"].([]any) {
		p := raw.(map[string]any)
		if p["name"] == "aiimage" {
			_, err = c.Checked("POST", "/api/plugin-switch", map[string]any{"name": "aiimage", "groupId": "0", "mode": "enabled", "revision": p["state"].(map[string]any)["revision"]})
			if err != nil {
				return err
			}
		}
	}
	cfg, err := c.Checked("GET", "/api/ai-image", nil)
	if err != nil {
		return err
	}
	_, err = c.Checked("POST", "/api/ai-image", map[string]any{"revision": cfg["revision"], "mode": "openai", "url": server.URL + "/v1/images/generations", "model": "gpt-image-2.5-sunburst", "quality": "high", "monthlyBudgetUSD": "0.2", "customCostUSD": "0.1", "key": map[string]any{"action": "replace", "value": "synthetic-key"}})
	if err != nil {
		return err
	}
	send := func(prompt, url, want string) error {
		segments := []map[string]any{{"type": "text", "data": map[string]string{"text": prompt}}}
		if url != "" {
			segments = append(segments, map[string]any{"type": "image", "data": map[string]string{"url": url, "file": "opaque.png"}})
		}
		reply, _, e := h.SendWithContext(ctx, qa.Owner, prompt, segments...)
		if e != nil {
			return e
		}
		if !strings.Contains(reply, want) {
			return fmt.Errorf("unexpected reply: %s", reply)
		}
		return nil
	}
	if err = send("AI画图帮助 参考图", "", "参考图 / 带图绘制"); err != nil {
		return err
	}
	if err = send("AI画图 帮我改图", server.URL+"/missing", "无法读取"); err != nil {
		return err
	}
	before, err := c.Checked("GET", "/api/ai-image", nil)
	if err != nil {
		return err
	}
	if before["budget"].(map[string]any)["chargedUSD"] != "0.000000" || edits.Load() != 0 {
		return fmt.Errorf("invalid image caused model call/budget charge")
	}
	if err = send("AI画图 保留主体，把背景换成蓝色", server.URL+"/source.png", "已读取1张参考图"); err != nil {
		return err
	}
	reply, err := h.AwaitReply(ctx)
	if err != nil || !strings.Contains(reply, "参考图: 1张") {
		return fmt.Errorf("reference result missing: %s %v", reply, err)
	}
	if err = send("AI画图 画一朵花", "", "正在生成"); err != nil {
		return err
	}
	if _, err = h.AwaitReply(ctx); err != nil {
		return err
	}
	if err = send("AI画图 再修改一次", server.URL+"/source.png", "预算不足"); err != nil {
		return err
	}
	if edits.Load() != 1 || generations.Load() != 1 || bad.Load() != 0 {
		return fmt.Errorf("wrong routing/pixels: edits=%d generations=%d bad=%d", edits.Load(), generations.Load(), bad.Load())
	}
	result := map[string]any{"passed": true, "isolated": true, "realQQMessagesSent": false, "realModelCalls": 0, "referenceBytesVerified": true, "referenceHelpVerified": true, "referenceRoutingVerified": true, "invalidImageNoCharge": true, "budgetStopVerified": true, "syntheticEditCalls": edits.Load(), "syntheticGenerationCalls": generations.Load()}
	b, _ := json.MarshalIndent(result, "", "  ")
	os.WriteFile(filepath.Join(parent, "integration-result.json"), b, 0600)
	fmt.Println(string(b))
	return nil
}
