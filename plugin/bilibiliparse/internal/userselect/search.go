package userselect

import (
	"encoding/json"
	"fmt"
	bz "github.com/FloatTech/AnimeAPI/bilibili"
	"io"
	"net/http"
	"net/url"
	"time"
)

func searchUser(cfg *bz.CookieConfig, keyword string) ([]bz.SearchResult, error) {
	client := &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	cookie := ""
	if cfg != nil {
		var err error
		cookie, err = cfg.Load()
		if err != nil {
			return nil, err
		}
	}
	return searchWithClient(client, keyword, cookie)
}
func searchWithClient(client *http.Client, keyword, cookie string) ([]bz.SearchResult, error) {
	endpoint := "https://api.bilibili.com/x/web-interface/search/type?search_type=bili_user&keyword=" + url.QueryEscape(keyword)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("B站用户搜索：请求无效")
	}
	req.Header.Set("User-Agent", bz.Ua)
	req.Header.Set("Referer", "https://www.bilibili.com/")
	req.Header.Set("Cookie", cookie)
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("B站用户搜索：请求失败或超时")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("B站用户搜索：HTTP %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, (2<<20)+1))
	if err != nil || len(data) > 2<<20 {
		return nil, fmt.Errorf("B站用户搜索：响应读取失败或过大")
	}
	var envelope struct {
		Code *int `json:"code"`
		Data struct {
			Result json.RawMessage `json:"result"`
		} `json:"data"`
	}
	if json.Unmarshal(data, &envelope) != nil || envelope.Code == nil {
		return nil, fmt.Errorf("B站用户搜索：响应格式无效")
	}
	if *envelope.Code != 0 {
		return nil, fmt.Errorf("B站用户搜索：接口错误（%d）", *envelope.Code)
	}
	var results []bz.SearchResult
	if len(envelope.Data.Result) == 0 || json.Unmarshal(envelope.Data.Result, &results) != nil {
		return nil, fmt.Errorf("B站用户搜索：结果列表无效")
	}
	for _, r := range results {
		if r.Mid <= 0 {
			return nil, fmt.Errorf("B站用户搜索：用户编号无效")
		}
	}
	return results, nil
}
