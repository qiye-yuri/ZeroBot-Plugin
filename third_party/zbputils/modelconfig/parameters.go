package modelconfig

import (
	"errors"
	"math"
	"strings"
	"unicode/utf8"
)

// Parameters contains only explicit overrides. Nil fields inherit global values;
// a non-nil empty reasoning string explicitly selects the protocol default.
type Parameters struct {
	MaxN            *uint    `json:"maxN,omitempty"`
	TopP            *float32 `json:"topP,omitempty"`
	ReasoningEffort *string  `json:"reasoningEffort,omitempty"`
}

type RequestParameters struct {
	MaxN            uint    `json:"maxN"`
	TopP            float32 `json:"topP"`
	ReasoningEffort string  `json:"reasoningEffort"`
}

func Defaults(max uint, top float32, reasoning string) RequestParameters {
	if max == 0 {
		max = 4096
	}
	if top == 0 {
		top = .9
	}
	return RequestParameters{MaxN: max, TopP: top, ReasoningEffort: reasoning}
}

func (p *Parameters) Resolve(global RequestParameters) RequestParameters {
	if p == nil {
		return global
	}
	if p.MaxN != nil {
		global.MaxN = *p.MaxN
	}
	if p.TopP != nil {
		global.TopP = *p.TopP
	}
	if p.ReasoningEffort != nil {
		global.ReasoningEffort = *p.ReasoningEffort
	}
	return global
}

func (p *Parameters) Validate() error {
	if p == nil {
		return nil
	}
	if p.MaxN != nil && (*p.MaxN < 1 || *p.MaxN > 131072) {
		return errors.New("模型最大输出长度须为 1～131072")
	}
	if p.TopP != nil && (math.IsNaN(float64(*p.TopP)) || math.IsInf(float64(*p.TopP), 0) || *p.TopP <= 0 || *p.TopP > 1) {
		return errors.New("模型 TopP 须大于 0 且不超过 1")
	}
	if p.ReasoningEffort != nil {
		text := *p.ReasoningEffort
		if len(text) > 256 || !utf8.ValidString(text) || strings.ContainsRune(text, '\x00') {
			return errors.New("模型推理设置过长或包含无效字符")
		}
	}
	return nil
}
