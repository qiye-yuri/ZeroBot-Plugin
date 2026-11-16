package imagegen

import "testing"

func TestOfficialCostDecimalEncodings(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{{"0E-6176", "0.000000000"}, {"0.0081920000000000000000000000", "0.008192000"}, {"1e-6", "0.000001000"}, {"-0.001", "-0.001000000"}} {
		v, e := ParseProviderUSD(tc.raw)
		if e != nil || v.FloatString(9) != tc.want {
			t.Fatal(tc, e)
		}
	}
	for _, raw := range []string{"1e999999999", "1e9999", "NaN", "", "secret"} {
		if _, e := ParseProviderUSD(raw); e == nil {
			t.Fatal(raw)
		}
	}
}
