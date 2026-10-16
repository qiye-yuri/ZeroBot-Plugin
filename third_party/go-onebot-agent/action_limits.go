package goba

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode/utf8"

	zero "github.com/wdvxdr1123/ZeroBot"
)

const (
	agentActionOutputBytes = 1 << 20
	agentActionObjects     = 32
	agentActionJSONDepth   = 64
	agentActionJSONValues  = 10000
)

var ErrActionOutputLimit = errors.New("Agent action output exceeds byte, object or structure limit")
var ErrActionOutputInvalid = errors.New("Agent action output is not a valid JSON object stream")

// Validate the whole admitted object stream before automatic writes or dispatch.
// Decode until EOF rather than More: More silently stops at stray ] or } tokens.
// The first end marker still terminates parsing; permission handling is unchanged.
func checkActionObjects(text string) error {
	if !utf8.ValidString(text) {
		return ErrActionOutputInvalid
	}
	dec := json.NewDecoder(strings.NewReader(text))
	budget := actionJSONBudget{}
	for count := 0; ; count++ {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return ErrActionOutputInvalid
		}
		if count == agentActionObjects {
			return ErrActionOutputLimit
		}
		var req zero.APIRequest
		if len(raw) == 0 || raw[0] != '{' {
			return ErrActionOutputInvalid
		}
		if err := budget.checkKeys(raw); err != nil {
			return err
		}
		reqDec := json.NewDecoder(bytes.NewReader(raw))
		reqDec.UseNumber() // Match execution parsing, including integers above 2^53.
		if reqDec.Decode(&req) != nil {
			return ErrActionOutputInvalid
		}
		if req.Action == EOA {
			return nil
		}
	}
}
