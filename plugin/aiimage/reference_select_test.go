package aiimage

import (
	"strings"
	"testing"
)

func TestSelectFirstReferenceArguments(t *testing.T) {
	for _, tc := range []struct {
		in, out string
		n       int
	}{{"--refs=2 --size=4k 修改", "--size=4k 修改", 2}, {"修改 --refs=1", "修改", 1}, {"--quality=high --refs=3 修改", "--quality=high 修改", 3}, {"修改", "修改", 0}} {
		out, n, err := parseReferenceCount(tc.in)
		if err != nil || out != tc.out || n != tc.n {
			t.Fatal(tc, out, n, err)
		}
	}
	for _, arg := range []string{"--refs=0 修改", "--refs=17 修改", "--refs=-1 修改", "--refs=1.5 修改", "--refs= 修改", "--refs 修改", "--refs=1 --refs=2 修改", "--refs=2"} {
		if _, _, err := parseReferenceCount(arg); err == nil {
			t.Fatal(arg)
		}
	}
	src := []referenceSource{{File: "first"}, {File: "second"}, {File: "bad-third"}}
	selected, err := selectFirstReferences(src, 2)
	if err != nil || len(selected) != 2 || selected[0].File != "first" || selected[1].File != "second" {
		t.Fatal(selected, err)
	}
	if _, err = selectFirstReferences(src, 4); err == nil {
		t.Fatal("missing references accepted")
	}
	if _, err = selectFirstReferences(nil, 1); err == nil {
		t.Fatal("no image accepted")
	}
	clean, _, err := parseReferenceCount("--refs=2 --n=3 修改")
	if err != nil {
		t.Fatal(err)
	}
	_, v, err := parseImageOptions(clean, defaultSettings())
	o, _ := optionsFor(v)
	if err != nil || o.Count != 3 || strings.Contains(clean, "--refs") {
		t.Fatal(clean, o, err)
	}
}
