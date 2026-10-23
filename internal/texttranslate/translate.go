// Package texttranslate provides bounded Chinese/English translation requests.
package texttranslate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var client = &http.Client{Timeout: 15 * time.Second}

// Translate translates Han-containing text to English, other text to Chinese.
func Translate(text string) (string, error) {
	return translate(client, "https://uapis.cn/api/v1/translate/text", text)
}

func translate(c *http.Client, endpoint, text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("请输入要翻译的文字")
	}
	if utf8.RuneCountInString(text) > 1000 {
		return "", fmt.Errorf("翻译文字不能超过1000字")
	}
	lang := "zh"
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			lang = "en"
			break
		}
	}
	body, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return "", err
	}
	rsp, err := c.Post(endpoint+"?to_lang="+lang, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("翻译服务 HTTP %d", rsp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(rsp.Body, 65537))
	if err != nil {
		return "", err
	}
	if len(raw) > 65536 {
		return "", fmt.Errorf("翻译服务响应过大")
	}
	var result struct {
		Translation string `json:"translate"`
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("翻译服务返回无效JSON")
	}
	if strings.TrimSpace(result.Translation) == "" {
		return "", fmt.Errorf("翻译服务未返回译文")
	}
	return result.Translation, nil
}
