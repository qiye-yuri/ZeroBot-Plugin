package imagegen

import (
	"fmt"
	"math/big"
	"regexp"
)

var providerMoney = regexp.MustCompile(`^-?[0-9]{1,12}(\.[0-9]{1,96})?([eE][+-]?[0-9]{1,4})?$`)

// The Costs API emits decimal zero as 0E-6176. Retain decimal precision without floats.
func ParseProviderUSD(s string) (*big.Rat, error) {
	if len(s) > 128 || !providerMoney.MatchString(s) {
		return nil, fmt.Errorf("官方费用金额格式无效")
	}
	value, ok := new(big.Rat).SetString(s)
	if !ok || new(big.Rat).Abs(value).Cmp(big.NewRat(1000000000, 1)) > 0 {
		return nil, fmt.Errorf("官方费用金额超出范围")
	}
	return value, nil
}
