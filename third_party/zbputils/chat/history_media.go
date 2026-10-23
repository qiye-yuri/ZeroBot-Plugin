package chat

import (
	"encoding/json"
	"strings"
)

// Compact only model-facing copies of structured media segments. Archives and
// current action execution keep their original payloads and numeric identifiers.
func compactHistoryMedia(raw string) string {
	if !strings.Contains(raw, "base64") {
		return raw
	}
	var visit func(json.RawMessage) (json.RawMessage, bool)
	visit = func(value json.RawMessage) (json.RawMessage, bool) {
		var object map[string]json.RawMessage
		if json.Unmarshal(value, &object) == nil && object != nil {
			changed := false
			var kind string
			_ = json.Unmarshal(object["type"], &kind)
			if kind == "image" || kind == "record" || kind == "video" {
				var data map[string]json.RawMessage
				if json.Unmarshal(object["data"], &data) == nil && data != nil {
					var file string
					_ = json.Unmarshal(data["file"], &file)
					if strings.HasPrefix(file, "base64://") || (strings.HasPrefix(file, "data:") && strings.Contains(file, ";base64,")) {
						data["file"], _ = json.Marshal("[历史媒体二进制已省略；不代表图片内容或可重发文件]")
						object["data"], _ = json.Marshal(data)
						changed = true
					}
				}
			}
			for key, child := range object {
				if next, ok := visit(child); ok {
					object[key] = next
					changed = true
				}
			}
			if changed {
				result, _ := json.Marshal(object)
				return result, true
			}
			return value, false
		}
		var array []json.RawMessage
		if json.Unmarshal(value, &array) == nil && array != nil {
			changed := false
			for i, child := range array {
				if next, ok := visit(child); ok {
					array[i] = next
					changed = true
				}
			}
			if changed {
				result, _ := json.Marshal(array)
				return result, true
			}
		}
		return value, false
	}
	if next, changed := visit(json.RawMessage(raw)); changed {
		return string(next)
	}
	return raw
}
