package gameflow

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestWords(t *testing.T) {
	got := Words("APPLE\r\n apple \n\n berry tool 中国abc a-bcd", 5)
	if !reflect.DeepEqual(got, []string{"apple", "berry"}) {
		t.Fatal(got)
	}
	if len(Words("\n bad\n", 7)) != 0 {
		t.Fatal("invalid dictionary accepted")
	}
}
func TestLocalDictionary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dict.txt")
	if err := os.WriteFile(path, []byte("local-only"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadData(path)
	if err != nil || string(got) != "local-only" {
		t.Fatal(string(got), err)
	}
}
