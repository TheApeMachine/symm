package broker

import (
	"strings"

	"github.com/theapemachine/symm/kraken/order"
)

func CashDeltaBuy(fill order.Fill, quoteCurrency string) float64 {
	cost := fill.Qty * fill.Price

	if fill.Fee <= 0 {
		return cost
	}

	if feeInQuote(fill.FeeCcy, quoteCurrency) {
		return cost + fill.Fee
	}

	return cost
}

func CashDeltaSell(fill order.Fill, quoteCurrency string) float64 {
	proceeds := fill.Qty * fill.Price

	if fill.Fee <= 0 {
		return proceeds
	}

	if feeInQuote(fill.FeeCcy, quoteCurrency) {
		return proceeds - fill.Fee
	}

	return proceeds
}

func feeInQuote(feeCurrency, quoteCurrency string) bool {
	if feeCurrency == "" {
		return true
	}

	return strings.EqualFold(feeCurrency, quoteCurrency)
}
