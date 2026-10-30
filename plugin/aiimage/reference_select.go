package aiimage

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var referenceCountArgument = regexp.MustCompile(`(?:^|\s)--refs(?:=([^\s]*))?(?:\s|$)`)

// Parse before get_msg/download/budget. This is input selection, not output n.
func parseReferenceCount(prompt string) (string, int, error) {
	count := 0
	seen := false
	var parseErr error
	// Split on whitespace boundaries without consuming the next argument's space.
	fields := strings.Fields(prompt)
	for _, field := range fields {
		if field != "--refs" && !strings.HasPrefix(field, "--refs=") {
			continue
		}
		if seen {
			return "", 0, fmt.Errorf("--refs只能填写一次")
		}
		seen = true
		value := strings.TrimPrefix(field, "--refs=")
		for _, c := range value {
			if c < '0' || c > '9' {
				parseErr = fmt.Errorf("--refs须为1–16的整数")
			}
		}
		n, e := strconv.Atoi(value)
		if e != nil || n < 1 || n > maxReferenceCount || parseErr != nil {
			return "", 0, fmt.Errorf("--refs须为1–16的整数，例如--refs=2")
		}
		count = n
	}
	if !seen {
		return prompt, 0, nil
	}
	clean := strings.TrimSpace(referenceCountArgument.ReplaceAllString(prompt, " "))
	if clean == "" {
		return "", 0, fmt.Errorf("请在--refs参数后填写修改要求")
	}
	return clean, count, nil
}
func selectFirstReferences(sources []referenceSource, count int) ([]referenceSource, error) {
	if count == 0 {
		return sources, nil
	}
	if count < 1 || count > maxReferenceCount {
		return nil, fmt.Errorf("--refs须为1–16的整数")
	}
	if len(sources) < count {
		return nil, fmt.Errorf("请求前%d张参考图，但当前图片来源只有%d张；请调整--refs", count, len(sources))
	}
	return sources[:count], nil
}
