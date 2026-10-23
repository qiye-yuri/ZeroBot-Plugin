package bilibiliparse

import "testing"

func TestFeatureBits(t *testing.T) {
	for name, bit := range featureBits {
		for _, fallback := range []bool{false, true} {
			if featureValue(0, bit, fallback) != fallback {
				t.Fatal(name, "lost legacy default")
			}
			original := enableVideoSummary | int64(0x20) // preserve the retired upload bit without using it
			on := setFeatureValue(original, bit, true)
			if !featureValue(on, bit, false) || on&original != original {
				t.Fatal(name, "enable damaged video settings")
			}
			off := setFeatureValue(on, bit, false)
			if featureValue(off, bit, true) || off&original != original {
				t.Fatal(name, "disable damaged video settings")
			}
			for other, otherBit := range featureBits {
				if other != name && featureValue(off, otherBit, true) != true {
					t.Fatal(name, "changed", other)
				}
			}
		}
	}
}
