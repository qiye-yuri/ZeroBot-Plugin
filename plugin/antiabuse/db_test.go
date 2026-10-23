package antiabuse

import (
	"path/filepath"
	"testing"
)

func TestLiteralWordsAndGroupIsolation(t *testing.T) {
	d, err := newantidb(filepath.Join(t.TempDir(), "words.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if d.insertWord(101, " \t") == nil {
		t.Fatal("empty word accepted")
	}
	if d.deleteWord(101, "") == nil {
		t.Fatal("empty deletion accepted")
	}
	for _, word := range []string{"a", "can't", "x;y", "' OR 1=1 --"} {
		if err := d.insertWord(101, word); err != nil {
			t.Fatal(err)
		}
		if !d.isInAntiList(101, "prefix "+word+" suffix") {
			t.Fatalf("literal did not match %q", word)
		}
		if d.isInAntiList(102, word) {
			t.Fatal("leaked to another group")
		}
	}
	if d.isInAntiList(101, "hello'world") {
		t.Fatal("quote changed SQL meaning")
	}
	if err := d.deleteWord(101, "' OR 1=1 --"); err != nil {
		t.Fatal(err)
	}
	if !d.isInAntiList(101, "can't") {
		t.Fatal("deletion removed unrelated words")
	}
	if d.isInAntiList(101, "' OR 1=1 --") {
		t.Fatal("literal deletion failed")
	}
	if err := d.insertWord(102, "a"); err != nil {
		t.Fatal(err)
	}
	if err := d.insertWord(102, "a"); err != nil {
		t.Fatal(err)
	}
	if got := d.listWords(102); got != "[a]" {
		t.Fatalf("single character/duplicate list: %q", got)
	}
	if err := d.insertWord(103, "placeholder"); err != nil {
		t.Fatal(err)
	}
	if err := d.Insert("2v", &banWord{Word: ""}); err != nil {
		t.Fatal(err)
	}
	if err := d.deleteWord(103, "placeholder"); err != nil {
		t.Fatal(err)
	}
	if d.isInAntiList(103, "anything") || d.listWords(103) != "[]" {
		t.Fatal("legacy empty word still active")
	}
}
