package aiimage

import (
	"embed"
	"strings"
)

//go:embed help/*.txt
var imageHelpFiles embed.FS

// Resolve help before configuration lookup and before budget/model operations.
func imageHelp(args string) ([]string, bool) {
	args = strings.TrimSpace(args)
	topic := ""
	switch {
	case args == "":
	case strings.HasPrefix(args, "帮助"):
		topic = strings.TrimSpace(strings.TrimPrefix(args, "帮助"))
	case args == "--help" || args == "help" || args == "说明":
	case strings.HasPrefix(args, "--help "):
		topic = strings.TrimSpace(strings.TrimPrefix(args, "--help"))
	case strings.HasPrefix(args, "help "):
		topic = strings.TrimSpace(strings.TrimPrefix(args, "help"))
	case args == "参数":
		topic = "参数"
	default:
		return nil, false
	}
	files := map[string]string{"": "overview", "总览": "overview", "参数": "parameters", "尺寸": "size", "质量": "quality", "输出": "output", "流式": "stream", "配置": "config", "预算": "budget", "示例": "examples", "参考图": "reference", "带图": "reference", "改图": "reference"}
	read := func(name string) string {
		b, _ := imageHelpFiles.ReadFile("help/" + name + ".txt")
		return strings.TrimSpace(strings.TrimPrefix(string(b), "\ufeff"))
	}
	if topic == "全部" {
		pages := []string{}
		for _, name := range []string{"overview", "parameters", "size", "quality", "output", "stream", "config", "budget", "reference", "examples"} {
			pages = append(pages, read(name))
		}
		return pages, true
	}
	if name, ok := files[topic]; ok {
		return []string{read(name)}, true
	}
	return []string{"未找到该帮助主题。请从以下入口选择：\n\n" + read("overview")}, true
}
