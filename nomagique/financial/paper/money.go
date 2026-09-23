package paper

import (
	"math/big"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
)

/*
The SDK Decimal keeps the left operand's scale, so a sum taken naively rounds
away digits the exchange reported, and its Mul/Div round through BankersRound,
which treats every exact quotient by 1 as a tie and moves odd results up
(99 × 1 at scale 0 is 100). Sums here widen the scale first and products are
taken on exact rationals, so money arithmetic is exact; only floorTo and
allocate round, and they round where the exchange itself would.
*/

func widest(left, right *decimal.Decimal) int64 {
	return max(left.GetScale(), right.GetScale())
}

func plus(left, right *decimal.Decimal) *decimal.Decimal {
	return left.SetScale(widest(left, right)).Add(right)
}

func minus(left, right *decimal.Decimal) *decimal.Decimal {
	return left.SetScale(widest(left, right)).Sub(right)
}

func times(left, right *decimal.Decimal) (*decimal.Decimal, error) {
	return exactly(new(big.Rat).Mul(left.Rat(), right.Rat()), left.GetScale()+right.GetScale())
}

/* percent is a percentage as a fraction, exactly. */
func percent(value *decimal.Decimal) (*decimal.Decimal, error) {
	return exactly(new(big.Rat).Quo(value.Rat(), big.NewRat(100, 1)), value.GetScale()+2)
}

func zero() *decimal.Decimal {
	return decimal.NewFromInt64(0)
}

func one() *decimal.Decimal {
	return decimal.NewFromInt64(1)
}

/* exactly renders a rational that terminates within scale digits. */
func exactly(value *big.Rat, scale int64) (*decimal.Decimal, error) {
	result, err := decimal.NewFromString(value.FloatString(int(scale)))

	if err != nil {
		return nil, errnie.Error(errnie.Err(errnie.Internal, "paper: render decimal", err))
	}
	return result, nil
}

/* floorTo is the largest multiple of increment not above value / divisor. */
func floorTo(value, divisor, increment *decimal.Decimal) (*decimal.Decimal, error) {
	units := new(big.Rat).Quo(value.Rat(), divisor.Rat())
	units.Quo(units, increment.Rat())
	count := new(big.Int).Quo(units.Num(), units.Denom())
	return exactly(new(big.Rat).Mul(new(big.Rat).SetInt(count), increment.Rat()), increment.GetScale())
}

/* allocate is whole's share for part of total, rounded half-even at whole's scale. */
func allocate(whole, part, total *decimal.Decimal) (*decimal.Decimal, error) {
	share := new(big.Rat).Mul(whole.Rat(), part.Rat())
	share.Quo(share, total.Rat())
	share.Mul(share, new(big.Rat).SetInt(whole.ScalingFactor()))
	units := decimal.BankersRound(new(big.Int).Set(share.Num()), share.Denom())
	return exactly(new(big.Rat).SetFrac(units, whole.ScalingFactor()), whole.GetScale())
}

/* amount parses an exchange or graph decimal that must be present and positive. */
func amount(text, field string) (*decimal.Decimal, error) {
	if text == "" {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "paper: "+field+" is required", nil))
	}
	value, err := decimal.NewFromString(text)

	if err != nil {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "paper: "+field+" is not a decimal", err))
	}

	if value.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "paper: "+field+" must be positive", nil))
	}
	return value, nil
}
