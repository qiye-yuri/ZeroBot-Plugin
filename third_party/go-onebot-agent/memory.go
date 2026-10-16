package goba

import (
	"errors"
	"strings"

	zero "github.com/wdvxdr1123/ZeroBot"
)

var (
	errEmptyMemory     = errors.New("empty memory")
	errMemoryHasReturn = errors.New("memory has \\r|\\n")
)

// MemoryStorage can be a db or just some files
type MemoryStorage interface {
	Save(grp int64, text string) error
	Load(grp int64) []string
}

// CheckedMemoryStorage allows persistent implementations to fail closed.
type CheckedMemoryStorage interface{ LoadChecked(int64) ([]string, error) }

func extractMemory(r *zero.APIRequest) (string, error) {
	txt, ok := r.Params["text"].(string)
	if !ok || txt == "" {
		return "", errEmptyMemory
	}
	for _, c := range txt {
		if c == '\r' || c == '\n' {
			return "", errMemoryHasReturn
		}
	}
	return txt, nil
}

func (ag *Agent) memoryof(grp int64) (string, error) {
	var mems []string
	if checked, ok := ag.mem.(CheckedMemoryStorage); ok {
		var err error
		mems, err = checked.LoadChecked(grp)
		if err != nil {
			return "", err
		}
	} else {
		mems = ag.mem.Load(grp)
	}
	if len(mems) == 0 {
		return "", nil
	}
	sb := strings.Builder{}
	for _, m := range mems {
		sb.WriteByte('\n')
		sb.WriteString(m)
	}
	return sb.String(), nil
}
