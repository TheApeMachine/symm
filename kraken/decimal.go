package kraken

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

/*
Float64 extracts the float64 representation of a Kraken Decimal directly.
*/
func Float64(d *decimal.Decimal) float64 {
	if d == nil {
		return 0
	}

	return d.Float64()
}
