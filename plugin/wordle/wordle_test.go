package wordle

import "testing"

func TestWordleRound(t *testing.T) {
	words[5] = struct {
		dict []string
		cet4 []string
	}{dict: []string{"apple", "berry"}, cet4: []string{"apple"}}
	game := newWordleGame("apple")
	if _, _, err := game("zzzzz"); err != errUnknownWord {
		t.Fatal(err)
	}
	if _, _, err := game("xx"); err != errLengthNotEnough {
		t.Fatal(err)
	}
	win, pic, err := game("APPLE")
	if !win || err != nil || len(pic) == 0 {
		t.Fatal(win, len(pic), err)
	}
	game = newWordleGame("apple")
	for i := 0; i < 6; i++ {
		_, _, err = game("berry")
	}
	if err != errTimesRunOut {
		t.Fatal(err)
	}
}
