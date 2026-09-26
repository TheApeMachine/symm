package core

import (
	"bytes"
	"math/big"
	"strings"

	"github.com/cockroachdb/apd/v3"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
)

/* ReadDecimal uses the venue SDK representation for every monetary operand. */
func ReadDecimal(raw []byte, operand string) (*decimal.Decimal, error) {
	text := string(bytes.Trim(bytes.TrimSpace(raw), `"`))
	if text == "" {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "decimal: "+operand+" did not arrive", nil))
	}
	// The installed SDK parses exponent notation through binary big.Float;
	// 1e-8 loses precision there. APD, already used by the storage dependency,
	// supplies exact fixed decimal text before constructing the SDK value.
	parsed, _, err := apd.NewFromString(text)
	if err != nil {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "decimal: invalid "+operand, err))
	}
	canonical := parsed.Text('f')
	magnitude := strings.TrimPrefix(canonical, "-")
	value, err := decimal.NewFromString(magnitude)
	if err != nil {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "decimal: invalid "+operand, err))
	}
	// Arithmetic nodes choose a sufficient scale for exact sums and products.
	// Quotients explicitly floor at the authored precision using the SDK hook.
	value = value.SetRounding(FloorDecimal)
	if parsed.Negative {
		value = value.Mul(decimal.NewFromInt64(-1))
	}
	return value, nil
}

/* FloorDecimal is the SDK rounding policy for amounts that must fit a budget. */
func FloorDecimal(value, scale *big.Int) *big.Int { return new(big.Int).Div(value, scale) }

/* WriteDecimal emits the SDK value as canonical JSON decimal text. */
func WriteDecimal(value *decimal.Decimal) ([]byte, error) {
	text := value.String()
	if strings.Contains(text, ".") {
		text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	}
	return []byte(text), nil
}
