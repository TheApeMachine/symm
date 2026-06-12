package trader

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/logic"
)

/*
EntrySlotFraction returns the wallet fraction for a new entry ranked against
open positions by entry confidence.
*/
func EntrySlotFraction(
	holdings *logic.Holdings,
	entryConfidence float64,
	tradingConfig config.TradingConfig,
) (float64, error) {
	if holdings == nil {
		return 0, errnie.Error(errors.New("trader: holdings are required for sizing"))
	}

	higherCount := holdings.StrictlyHigherConfidenceCount(entryConfidence)

	if higherCount < tradingConfig.PrimarySlotCount {
		return tradingConfig.PrimarySlotFraction, nil
	}

	return tradingConfig.SecondarySlotFraction, nil
}

/*
OrderQuantityFromFraction converts quote wallet notional into base quantity.
*/
func OrderQuantityFromFraction(
	walletQuote float64,
	fraction float64,
	price float64,
) (float64, error) {
	if walletQuote <= 0 {
		return 0, errnie.Error(errors.New("trader: wallet quote balance must be positive"))
	}

	if fraction <= 0 {
		return 0, errnie.Error(errors.New("trader: position fraction must be positive"))
	}

	if price <= 0 {
		return 0, errnie.Error(errors.New("trader: reference price must be positive"))
	}

	notional := walletQuote * fraction
	quantity := notional / price

	if quantity <= 0 {
		return 0, errnie.Error(fmt.Errorf(
			"trader: computed quantity must be positive (wallet=%.4f fraction=%.4f price=%.4f)",
			walletQuote,
			fraction,
			price,
		))
	}

	return quantity, nil
}

/*
QuoteWalletBalance returns the configured paper quote balance for sizing.
*/
func QuoteWalletBalance(model string) (float64, error) {
	if model != "paper" {
		return 0, errnie.Error(fmt.Errorf(
			"trader: wallet sizing is only wired for paper model, got %q",
			model,
		))
	}

	quote := strings.ToLower(viper.GetString("market.quote_currency"))
	balance := viper.GetFloat64("trading.paper.wallet." + quote)

	if balance <= 0 {
		return 0, errnie.Error(fmt.Errorf(
			"trader: trading.paper.wallet.%s must be positive",
			quote,
		))
	}

	return balance, nil
}
