package modelconfig

import (
	"math"
	"testing"
)

func TestParameterFiniteBoundsAndEmptyReasoning(t *testing.T) {
	for _, top := range []float32{0, -.1, 1.01, float32(math.NaN()), float32(math.Inf(1))} {
		if err := (&Parameters{TopP: &top}).Validate(); err == nil {
			t.Fatal("invalid TopP", top)
		}
	}
	for _, max := range []uint{0, 131073} {
		if err := (&Parameters{MaxN: &max}).Validate(); err == nil {
			t.Fatal("invalid maximum", max)
		}
	}
	max, top, empty := uint(1), float32(1), ""
	p := &Parameters{MaxN: &max, TopP: &top, ReasoningEffort: &empty}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := p.Resolve(Defaults(0, 0, "high")); got.MaxN != 1 || got.TopP != 1 || got.ReasoningEffort != "" {
		t.Fatal(got)
	}
}
