package aiimage

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"unicode"
)

const maxImageErrorBody = 64 << 10

var imageErrorSensitive = regexp.MustCompile(`(?i)(?:data:image/[^\s"'<>]+|base64://[^\s"'<>]+|https?://[^\s"'<>]+|sk-[a-z0-9_-]+|bearer\s+[^\s"'<>]+|[a-z0-9+/=_-]{80,})`)

// Only bounded, structured provider error fields are exposed, never the raw body.
func imageAPIError(resp *http.Response, cfg imageConfig, prompt string, refs []referenceImage) error {
	clean := func(value string, limit int) string {
		for _, secret := range []string{cfg.APIKey, prompt} {
			if secret != "" {
				value = strings.ReplaceAll(value, secret, "[已隐藏]")
			}
		}
		for _, ref := range refs {
			if ref.DataURL != "" {
				value = strings.ReplaceAll(value, ref.DataURL, "[图片已隐藏]")
			}
		}
		value = imageErrorSensitive.ReplaceAllString(value, "[已隐藏]")
		value = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return ' '
			}
			return r
		}, value)
		// Prevent provider text from creating OneBot CQ segments.
		value = strings.NewReplacer("[", "［", "]", "］").Replace(value)
		runes := []rune(strings.TrimSpace(value))
		if len(runes) > limit {
			return string(runes[:limit]) + "…"
		}
		return string(runes)
	}
	parts := []string{fmt.Sprintf("接口返回HTTP %d", resp.StatusCode)}
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, maxImageErrorBody+1))
	var envelope struct {
		Error struct {
			Message string          `json:"message"`
			Type    string          `json:"type"`
			Code    json.RawMessage `json:"code"`
			Param   string          `json:"param"`
		} `json:"error"`
	}
	if readErr == nil && len(raw) <= maxImageErrorBody && json.Unmarshal(raw, &envelope) == nil {
		e := envelope.Error
		for _, field := range []struct {
			name, value string
			limit       int
		}{{"原因", e.Message, 600}, {"类型", e.Type, 100}, {"参数", e.Param, 100}} {
			if v := clean(field.value, field.limit); v != "" {
				parts = append(parts, field.name+": "+v)
			}
		}
		var code any
		decoder := json.NewDecoder(strings.NewReader(string(e.Code)))
		decoder.UseNumber()
		if decoder.Decode(&code) == nil {
			switch v := code.(type) {
			case string:
				if v != "" {
					parts = append(parts, "错误码: "+clean(v, 100))
				}
			case json.Number:
				parts = append(parts, "错误码: "+clean(v.String(), 100))
			}
		}
	}
	if len(parts) == 1 {
		parts = append(parts, "接口未提供可读取的结构化错误详情")
	}
	if id := clean(resp.Header.Get("x-request-id"), 160); id != "" {
		parts = append(parts, "请求编号: "+id)
	}
	return fmt.Errorf("%s", strings.Join(parts, "；"))
}
