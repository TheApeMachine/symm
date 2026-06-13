package trader

import (
	"context"
	"errors"
	"fmt"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/logic"
)

/*
CapitalProvider supplies quote capital for order sizing.
*/
type CapitalProvider interface {
	QuoteBalance(ctx context.Context, quote string) (float64, error)
	AvailableQuoteBalance(ctx context.Context, quote string) (float64, error)
}

/*
StaticCapitalProvider uses a fixed quote balance for deterministic paper sizing.
Paper trading assumes a single configured quote currency, so QuoteBalance and
AvailableQuoteBalance intentionally ignore the quote parameter.
*/
type StaticCapitalProvider struct {
	quoteBalance float64
}

func NewStaticCapitalProvider(quoteBalance float64) (*StaticCapitalProvider, error) {
	if quoteBalance <= 0 {
		return nil, errnie.Error(errors.New("trader: quote balance must be positive"))
	}

	return &StaticCapitalProvider{quoteBalance: quoteBalance}, nil
}

func NewCapitalProvider(
	tradingConfig config.TradingConfig,
) (CapitalProvider, error) {
	switch tradingConfig.Model {
	case "paper":
		quoteBalance, err := config.PaperWalletBalance()

		if err != nil {
			return nil, errnie.Error(err)
		}

		return NewStaticCapitalProvider(quoteBalance)
	case "live":
		return nil, errnie.Error(errors.New(
			"trader: live capital provider not configured",
		))
	default:
		return nil, errnie.Error(fmt.Errorf(
			"trader: unsupported trading model %q",
			tradingConfig.Model,
		))
	}
}

func (provider *StaticCapitalProvider) QuoteBalance(
	context.Context,
	string,
) (float64, error) {
	return provider.available()
}

func (provider *StaticCapitalProvider) AvailableQuoteBalance(
	context.Context,
	string,
) (float64, error) {
	return provider.available()
}

func (provider *StaticCapitalProvider) available() (float64, error) {
	if provider == nil || provider.quoteBalance <= 0 {
		return 0, errnie.Error(errors.New("trader: quote balance must be positive"))
	}

	return provider.quoteBalance, nil
}

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

	return config.PaperWalletBalance()
}
