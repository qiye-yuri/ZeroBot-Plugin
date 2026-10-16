package goba

import (
	"strings"
	"unicode/utf8"
)

const agentImageDescriptionBytes = 4000

type imageDescription struct {
	Text      string
	Truncated bool
}

func normalizeImageDescription(text string) (imageDescription, bool) {
	if !utf8.ValidString(text) || strings.ContainsRune(text, 0) {
		return imageDescription{}, false
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return imageDescription{}, false
	}
	desc := imageDescription{Text: text}
	if len(text) > agentImageDescriptionBytes {
		end := agentImageDescriptionBytes
		for !utf8.ValidString(text[:end]) {
			end--
		}
		desc.Text, desc.Truncated = text[:end], true
	}
	return desc, true
}

func (d imageDescription) apply(data map[string]string) {
	data["__agent_desc__"] = d.Text
	delete(data, "__agent_desc_truncated__")
	if d.Truncated {
		data["__agent_desc_truncated__"] = "true"
	}
}
