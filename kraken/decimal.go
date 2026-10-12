package kraken

import (
	"math/big"
	"unsafe"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

type rawDecimal struct {
	integer   *big.Int
	increment int64
	scale     int64
}

var pow10 = [...]float64{
	1e0, 1e1, 1e2, 1e3, 1e4, 1e5, 1e6, 1e7, 1e8, 1e9,
	1e10, 1e11, 1e12, 1e13, 1e14, 1e15, 1e16, 1e17, 1e18,
}

/*
Float64 extracts the float64 representation of a Kraken Decimal directly.
For decimals whose unscaled integer fits in an int64 and scale <= 18, it computes
the float64 value directly without allocating big.Rat, ScalingFactor or strings.
*/
func Float64(d *decimal.Decimal) float64 {
	if d == nil {
		return 0
	}

	raw := (*rawDecimal)(unsafe.Pointer(d))

	if raw.integer == nil {
		return 0
	}

	scale := raw.scale

	if scale >= 0 && scale <= 18 && raw.integer.IsInt64() {
		return float64(raw.integer.Int64()) / pow10[scale]
	}

	value, _ := d.Rat().Float64()
	return value
}
