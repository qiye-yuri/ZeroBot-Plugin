// Package gameflow shares resource and answer handling between guessing modes.
package gameflow

import (
	"github.com/FloatTech/floatbox/file"
	"github.com/FloatTech/zbputils/control"
	zero "github.com/wdvxdr1123/ZeroBot"
	"os"
	"regexp"
	"strings"
)

// LoadData preserves existing local dictionaries and fetches only missing files.
func LoadData(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err == nil || !os.IsNotExist(err) {
		return data, err
	}
	return file.GetLazyData(path, control.Md5File, false)
}

func Words(raw string, size int) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, word := range strings.Fields(strings.ToLower(raw)) {
		if len(word) != size || seen[word] {
			continue
		}
		valid := true
		for _, c := range word {
			if c < 'a' || c > 'z' {
				valid = false
				break
			}
		}
		if valid {
			result = append(result, word)
			seen[word] = true
		}
	}
	return result
}

func AnswerRule(start *zero.Ctx, pattern string) zero.Rule {
	re := regexp.MustCompile(pattern)
	return func(ctx *zero.Ctx) bool {
		if ctx.Event.UserID == ctx.Event.SelfID {
			return false
		}
		text := ctx.ExtractPlainText()
		if text == "结束猜词" {
			return ctx.Event.UserID == start.Event.UserID || zero.AdminPermission(ctx)
		}
		return re.MatchString(text)
	}
}
