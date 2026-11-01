package broker

import (
	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	sdkdecimal "github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/decimal"
)

func dFloat(value float64) *decimal.Decimal {
	return decimal.NewFromFloat64(value)
}

func dInt(value int64) *decimal.Decimal {
	return decimal.NewFromInt64(value)
}

/*
toSDK converts a desk decimal for Kraken SDK book writes.
*/
func toSDK(value *decimal.Decimal) (*sdkdecimal.Decimal, error) {
	if value == nil {
		return nil, nil
	}

	return sdkdecimal.NewFromString(value.String())
}

/*
fromSDK converts a Kraken SDK decimal into a desk decimal.
*/
func fromSDK(value *sdkdecimal.Decimal) (*decimal.Decimal, error) {
	if value == nil {
		return nil, nil
	}

	return decimal.NewFromString(value.String())
}

func mustFromSDK(value *sdkdecimal.Decimal) *decimal.Decimal {
	parsed, err := fromSDK(value)
	if err != nil {
		return nil
	}

	return parsed
}

func mustToSDK(value *decimal.Decimal) *sdkdecimal.Decimal {
	parsed, err := toSDK(value)
	if err != nil {
		return nil
	}

	return parsed
}

/*
balanceMapFromSDK converts a REST balance map at the SDK boundary.
*/
func balanceMapFromSDK(values map[string]*sdkdecimal.Decimal) map[string]*decimal.Decimal {
	out := make(map[string]*decimal.Decimal, len(values))

	for asset, amount := range values {
		out[asset] = mustFromSDK(amount)
	}

	return out
}

func levelPrice(level *spotbook.Level) *decimal.Decimal {
	if level == nil {
		return nil
	}

	return mustFromSDK(level.Price)
}

func levelQty(level *spotbook.Level) *decimal.Decimal {
	if level == nil {
		return nil
	}

	return mustFromSDK(level.Quantity)
}

func formatSize(normalizer *spot.Normalizer, symbol string, value *decimal.Decimal) (*decimal.Decimal, error) {
	if normalizer == nil {
		return value, nil
	}

	sized, err := normalizer.FormatSize(symbol, mustToSDK(value))
	if err != nil {
		return nil, err
	}

	return mustFromSDK(sized), nil
}
