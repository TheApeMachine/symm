package cmd

import (
	"context"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/system"
)

/*
offlinePrice builds the offline friction model shared by offline commands:
Kraken's public pair facts for lot normalization, the explicit taker fee
(percent per side) for every requested symbol, and the configured account
balance as reference cash for allocation sizing.
*/
func offlinePrice(ctx context.Context, feePercent string, symbols []string) (*broker.Price, error) {
	fee, err := decimal.NewFromString(feePercent)

	if err != nil || fee == nil || fee.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"--taker-fee-percent must be a positive decimal percent per side (e.g. 0.26)",
			err,
		))
	}

	if system.Cfg.Market.Balance <= 0 {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "market.balance must be positive", nil))
	}

	normalizer := spot.NewNormalizer()

	if err := broker.SeedNormalizer(normalizer); err != nil {
		return nil, err
	}

	price := broker.NewPrice(ctx, nil, nil, nil, normalizer)
	price.SetDefaultFee(kraken.TradeVolumeFee{Fee: fee})

	for _, symbol := range symbols {
		price.SetFee(symbol, kraken.TradeVolumeFee{Fee: fee})

		if price.Fee(symbol) == nil {
			return nil, errnie.Error(errnie.Err(errnie.NotFound, "fee did not register for "+symbol, nil))
		}
	}

	budget := decimal.NewFromInt64(int64(system.Cfg.Market.Balance))

	if system.Cfg.Risk != nil && system.Cfg.Risk.MaxFraction != nil && system.Cfg.Risk.MaxFraction.Sign() > 0 {
		budget = budget.Mul(system.Cfg.Risk.MaxFraction)
	}

	price.SetReferenceCash(budget)

	return price, nil
}
