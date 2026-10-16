package goba

import (
	"errors"
	"strings"
	"testing"
)

func TestActionDuplicateKeys(t *testing.T) {
	for _, tc := range []struct {
		name, output string
		invalid      bool
	}{
		{"action", `{"action":"send_group_msg","action":"end_action"}`, true},
		{"escaped-action", `{"action":"send_group_msg","act\u0069on":"end_action"}`, true},
		{"case-action", `{"action":"send_group_msg","Action":"end_action"}`, true},
		{"params-merge", `{"action":"send_group_msg","params":{"group_id":10},"Params":{"message":"x"}}`, true},
		{"unicode-case-params", `{"params":{},"paramſ":{}}`, true},
		{"echo", `{"echo":1,"ECHO":2}`, true},
		{"target", `{"action":"send_group_msg","params":{"group_id":10,"group_id":20}}`, true},
		{"escaped-target", `{"action":"send_group_msg","params":{"group_id":10,"group\u005fid":20}}`, true},
		{"same-value-still-duplicate", `{"action":"send_group_msg","params":{"group_id":10,"group_id":10}}`, true},
		{"memory-text", `{"action":"save_memory","params":{"text":"a","text":"b"}}`, true},
		{"message-segment", `{"action":"send_group_msg","params":{"message":[{"type":"text","data":{"text":"a","text":"b"}}]}}`, true},
		{"unknown-field", `{"action":"end_action","extra":{},"extra":[]}`, true},
		{"empty-objects", `{"action":"end_action","params":{},"extra":{},"echo":0}`, false},
		{"sibling-keys", `{"action":"send_group_msg","params":{"message":[{"type":"text","data":{"text":"a"}},{"type":"text","data":{"text":"b"}}]}}`, false},
		{"map-case-sensitive", `{"action":"send_group_msg","params":{"group_id":10,"GROUP_ID":20}}`, false},
		{"single-case-alias", `{"Action":"end_action","Params":{},"Echo":9007199254740993}`, false},
		{"literal-text", `{"action":"send_group_msg","params":{"message":"{\"text\":1,\"text\":2}"}}`, false},
		{"independent-actions", `{"action":"send_group_msg","params":{"group_id":10}}` + "\n" + `{"action":"end_action","params":{}}`, false},
		{"after-first-end-compatible", `{"action":"end_action"}{"action":"a","action":"b"}`, false},
		{"bounded-depth-compatible", `{"action":"send_group_msg","params":{"extra":` + strings.Repeat("[", 62) + "9007199254740993" + strings.Repeat("]", 62) + `}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := checkActionObjects(tc.output)
			if (tc.invalid && !errors.Is(err, ErrActionOutputInvalid)) || (!tc.invalid && err != nil) {
				t.Fatal("unexpected duplicate key validation", err)
			}
		})
	}
}
