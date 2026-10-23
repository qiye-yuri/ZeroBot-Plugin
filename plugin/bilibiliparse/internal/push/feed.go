package push

import (
	"encoding/json"
	"fmt"
	"github.com/tidwall/gjson"
	"io"
	"net/http"
	"sort"
	"strconv"
	"time"
)

func newFeedClient() *http.Client {
	return &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
func fetchDynamicFeed(client *http.Client, endpoint, cookie string) ([]gjson.Result, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("B站动态接口：请求无效")
	}
	req.Header.Set("Cookie", cookie)
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Referer", "https://www.bilibili.com/")
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("B站动态接口：请求失败或超时")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("B站动态接口：HTTP %d", res.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, (4<<20)+1))
	if err != nil || len(raw) > 4<<20 {
		return nil, fmt.Errorf("B站动态接口：响应读取失败或过大")
	}
	return decodeDynamicFeed(raw)
}
func decodeDynamicFeed(raw []byte) ([]gjson.Result, error) {
	if !json.Valid(raw) {
		return nil, fmt.Errorf("B站动态接口：响应格式无效")
	}
	root := gjson.ParseBytes(raw)
	code := root.Get("code")
	if code.Type != gjson.Number || code.Int() != 0 {
		return nil, fmt.Errorf("B站动态接口：业务错误（%s）", code.Raw)
	}
	list := root.Get("data.items")
	if !list.IsArray() {
		return nil, fmt.Errorf("B站动态接口：缺少动态列表")
	}
	items := list.Array()
	for _, item := range items {
		id := item.Get("id_str").String()
		n, err := strconv.ParseUint(id, 10, 64)
		if err != nil || n == 0 || item.Get("modules.module_author.pub_ts").Int() <= 0 || item.Get("modules.module_author.name").String() == "" {
			return nil, fmt.Errorf("B站动态接口：动态字段不完整")
		}
	}
	// Pinned older items need not be first chronologically.
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Get("modules.module_author.pub_ts").Int() > items[j].Get("modules.module_author.pub_ts").Int()
	})
	return items, nil
}

func pendingDynamics(items []gjson.Result, last int64, initialized bool, now int64) ([]gjson.Result, int64) {
	newest := last
	var pending []gjson.Result
	seen := map[string]bool{}
	for _, item := range items {
		ts := item.Get("modules.module_author.pub_ts").Int()
		if ts > newest {
			newest = ts
		}
		id := item.Get("id_str").String()
		if initialized && ts > last && ts > now-600 && !seen[id] {
			pending = append(pending, item)
			seen[id] = true
		}
	}
	sort.SliceStable(pending, func(i, j int) bool {
		return pending[i].Get("modules.module_author.pub_ts").Int() < pending[j].Get("modules.module_author.pub_ts").Int()
	})
	return pending, newest
}
