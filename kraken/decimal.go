package kraken

import (
	"math"
	"math/big"
	"unsafe"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

type decimalLayout struct {
	integer   *big.Int
	increment int64
	scale     int64
	rounding  uintptr
}

var pow10Table = [...]float64{
	1e0, 1e1, 1e2, 1e3, 1e4, 1e5, 1e6, 1e7, 1e8, 1e9,
	1e10, 1e11, 1e12, 1e13, 1e14, 1e15, 1e16, 1e17, 1e18,
}

/*
Float64 extracts the float64 representation of a Kraken Decimal without
allocating big.Rat or big.Int structures on the heap.
*/
func Float64(d *decimal.Decimal) float64 {
	if d == nil {
		return 0
	}

	layout := (*decimalLayout)(unsafe.Pointer(d))
	if layout.integer == nil || layout.integer.Sign() == 0 {
		return 0
	}

	scale := layout.scale
	if layout.integer.IsInt64() {
		val := float64(layout.integer.Int64())
		if scale >= 0 && int(scale) < len(pow10Table) {
			return val / pow10Table[scale]
		}
		return val / math.Pow10(int(scale))
	}

	return d.Float64()
}
