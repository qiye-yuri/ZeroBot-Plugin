package goba

import (
	_ "embed"
	"fmt"
	"time"
	"unicode/utf8"
)

//go:embed README.md
var sysp string

// Config stores mutable characteristics of the agent.
type Config struct {
	Nickname  string // Nickname 昵称
	Sex       string // Sex 性别
	Chars     string // Chars 个性
	WebSearch bool   // Per-task host permission; never enabled by conversation text.
}

func (ag *Agent) system(role PermRole, iter int, grp int64, characters ...Config) (string, error) {
	character := *ag.cfg
	if len(characters) > 0 {
		character = characters[0]
	}
	tab, err := ag.perm.mdtable(role)
	if err != nil {
		return "", err
	}
	if character.WebSearch {
		a, ok := ag.perm.Actions[SearchWeb]
		if !ok {
			return "", ErrUnexpectedAction
		}
		tab += "\n|" + a.Desc + "|" + SearchWeb + "|" + a.Params + "|" + a.Data + "|"
	}
	t := time.Now()
	typ := "群聊"
	if grp < 0 {
		typ = "私聊"
	}
	memory, err := ag.memoryof(grp)
	if err != nil {
		return "", err
	}
	if len(memory) > 32768 {
		start := len(memory) - 32768
		for start < len(memory) && !utf8.ValidString(memory[start:]) {
			start++
		}
		memory = "（更早记忆未装载到本次请求，原文件保留）\n" + memory[start:]
	}
	return fmt.Sprintf(
		sysp, character.Nickname, character.Sex,
		character.Chars, tab, memory,
		t.Format(time.RFC3339), t.Unix(), typ, iter,
	), nil
}
