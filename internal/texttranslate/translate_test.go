package texttranslate

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTranslate(t *testing.T) {
	for _, tc := range []struct {
		name, input, lang, body, want string
		status                        int
	}{
		{"english", "good", "zh", `{"translate":"好"}`, "好", 200},
		{"chinese", "好", "en", `{"translate":"good"}`, "good", 200},
		{"escaping", "a & b?\n\"c\"", "zh", `{"translate":"测试"}`, "测试", 200},
		{"http", "test", "zh", `{}`, "", 429},
		{"invalid", "test", "zh", `<html>`, "", 200},
		{"empty", "test", "zh", `{"translate":" "}`, "", 200},
		{"oversize", "test", "zh", strings.Repeat("x", 65537), "", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Query().Get("to_lang") != tc.lang {
					t.Error("wrong method/target language")
				}
				var in map[string]string
				if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in["text"] != tc.input {
					t.Error("request text not preserved")
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer s.Close()
			got, err := translate(s.Client(), s.URL, tc.input)
			if tc.want == "" {
				if err == nil || got != "" {
					t.Fatalf("got %q, %v", got, err)
				}
			} else if err != nil || got != tc.want {
				t.Fatalf("got %q, %v", got, err)
			}
		})
	}
}

func TestRejectInputBeforeRequest(t *testing.T) {
	for _, input := range []string{" ", strings.Repeat("字", 1001)} {
		if _, err := translate(nil, "", input); err == nil {
			t.Fatal("invalid input accepted")
		}
	}
}
