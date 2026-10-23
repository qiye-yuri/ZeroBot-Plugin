package event

import (
	"encoding/binary"
	base14 "github.com/fumiama/go-base16384"
	zero "github.com/wdvxdr1123/ZeroBot"
	"testing"
)

func TestOpaqueFlags(t *testing.T) {
	for _, kind := range []string{"申请", "邀请"} {
		for _, flag := range []string{"0", "000123", "18446744073709551615", "opaque/中文+_-"} {
			got, err := decodeFlag(kind, encodeFlag(kind, flag))
			if err != nil || got != flag {
				t.Fatalf("roundtrip %q: %q %v", flag, got, err)
			}
		}
	}
	for _, token := range []string{"", "f_", "f_!", "g_YQ", "abcd"} {
		if _, err := decodeFlag("申请", token); err == nil {
			t.Errorf("accepted invalid %q", token)
		}
	}
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], 123456)
	if got, err := decodeFlag("申请", base14.EncodeToString(b[1:])); err != nil || got != "123456" {
		t.Fatal(got, err)
	}
}
func TestMissingOwner(t *testing.T) {
	old := zero.BotConfig.SuperUsers
	defer func() { zero.BotConfig.SuperUsers = old }()
	zero.BotConfig.SuperUsers = nil
	if hasOwner(nil) {
		t.Fatal("missing owner accepted")
	}
}
