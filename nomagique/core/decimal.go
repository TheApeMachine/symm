package core

import (
	"bytes"
	"math/big"

	"github.com/theapemachine/errnie"
)

/*
ReadDecimal reads a decimal travelling as JSON number text, bare or quoted as
exchanges write it, into an exact rational. Decimal arithmetic stays exact
this way; the exchange SDK's Decimal rounds odd results at scale 0 and is not
used for arithmetic.
*/
func ReadDecimal(raw []byte, operand string) (*big.Rat, error) {
	text := string(bytes.Trim(bytes.TrimSpace(raw), `"`))

	if text == "" {
		return nil, errnie.Err(
			errnie.Validation,
			"decimal: "+operand+" did not arrive",
			nil,
		)
	}

	value, ok := new(big.Rat).SetString(text)

	if !ok {
		return nil, errnie.Err(
			errnie.Validation,
			"decimal: "+operand+" is not a decimal: "+text,
			nil,
		)
	}

	return value, nil
}

/*
WriteDecimal renders a terminating rational with the fewest
places that hold it exactly.
*/
func WriteDecimal(value *big.Rat) ([]byte, error) {
	denominator := new(big.Int).Set(value.Denom())
	places := 0
	one := big.NewInt(1)

	for _, factor := range []*big.Int{big.NewInt(2), big.NewInt(5)} {
		count := 0
		remainder := new(big.Int)

		for denominator.Cmp(one) != 0 && remainder.Mod(
			denominator, factor,
		).Sign() == 0 {
			denominator.Quo(denominator, factor)
			count++
		}

		places = max(places, count)
	}

	if denominator.Cmp(one) != 0 {
		return nil, errnie.Err(
			errnie.Validation,
			"decimal: "+value.String()+" has no finite decimal form",
			nil,
		)
	}

	return []byte(value.FloatString(places)), nil
}
