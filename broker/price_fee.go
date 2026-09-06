package broker

import (
	"context"
	"fmt"
	"math/big"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
	"golang.org/x/sync/errgroup"
)

/* Fee returns the taker fee for a symbol. */
func (price *Price) Fee(symbol string) *kraken.TradeVolumeFee {
	fee := price.FeeIfAvailable(symbol)

	if fee == nil {
		errnie.Error(errnie.Err(
			errnie.NotFound,
			"fee not found for "+symbol,
			nil,
		))
	}

	return fee
}

/*
FeeIfAvailable returns the taker fee for a symbol, or nil when the fee
surface has not loaded it yet. It is the non-logging probe: consumers that
treat a missing fee as an unavailable state (rather than an error) use it so
availability checks do not flood the log.
*/
func (price *Price) FeeIfAvailable(symbol string) *kraken.TradeVolumeFee {
	if price == nil {
		return nil
	}

	found, ok := price.fees.Load(price.api.Normalizer().Name(symbol))

	if !ok {
		return nil
	}

	fee, ok := found.(kraken.TradeVolumeFee)

	if !ok {
		return nil
	}

	return &fee
}

/*
GetFees loads a complete TradeVolume taker-fee batch. Kraken keys these rows
with compact REST identifiers, which must be matched to the requested pair
rather than reparsed as an unseparated asset name.
*/
func (price *Price) GetFees(symbols []string) error {
	errnie.Info(fmt.Sprintf("getting fees for %d symbols", len(symbols)))

	tradeVolumeResult, err := price.api.TradeVolume(symbols)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"trade volume: failed to fetch",
			err,
		))
	}

	if tradeVolumeResult == nil {
		return errnie.Err(
			errnie.UnprocessableContent,
			"trade volume: response is required",
			nil,
		)
	}

	group, _ := errgroup.WithContext(context.Background())
	group.SetLimit(types.ShardWorkers())

	type feeResult struct {
		symbolKey string
		fee       kraken.TradeVolumeFee
	}
	results := make([]feeResult, len(symbols))

	for i, symbol := range symbols {
		i, symbol := i, symbol
		group.Go(func() error {
			fee, err := price.resolveFee(symbol, tradeVolumeResult.Fees)

			if err != nil {
				return err
			}

			results[i] = feeResult{
				symbolKey: price.normalizer.Name(symbol),
				fee:       fee,
			}

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return err
	}

	for _, r := range results {
		price.fees.Store(r.symbolKey, r.fee)
	}

	price.status = types.READY
	return nil
}

/* resolveFee matches one requested websocket symbol to Kraken's REST keys. */
func (price *Price) resolveFee(
	symbol string,
	fees map[string]kraken.TradeVolumeFee,
) (kraken.TradeVolumeFee, error) {
	pair, err := price.normalizer.PairInfo(symbol)

	if err != nil {
		return kraken.TradeVolumeFee{}, errnie.Err(
			errnie.Validation,
			"trade volume: pair metadata required for "+symbol,
			err,
		)
	}

	base, quote, found := price.normalizer.PairName(symbol)

	if !found {
		return kraken.TradeVolumeFee{}, errnie.Err(
			errnie.Validation,
			"trade volume: pair names required for "+symbol,
			nil,
		)
	}

	for _, identifier := range []string{
		pair.AltName,
		base.OldName + quote.OldName,
		base.Name + quote.Name,
	} {
		fee, ok := fees[identifier]

		if !ok {
			continue
		}

		if fee.Fee == nil || fee.Fee.Sign() < 0 {
			return kraken.TradeVolumeFee{}, errnie.Err(
				errnie.UnprocessableContent,
				"trade volume: invalid taker fee for "+symbol,
				nil,
			)
		}

		return fee, nil
	}

	return kraken.TradeVolumeFee{}, errnie.Err(
		errnie.NotFound,
		"trade volume: taker fee missing for "+symbol,
		nil,
	)
}

/* WithFee applies the symbol's taker fee in the requested direction. */
func (price *Price) WithFee(
	symbol string,
	amount *decimal.Decimal,
	direction Direction,
) *decimal.Decimal {
	fee := price.Fee(symbol)

	if fee == nil || fee.Fee == nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"price: taker fee required for fee calculation",
			nil,
		))

		return nil
	}

	if err := errnie.Error(errnie.Require(map[string]any{
		"symbol":    symbol,
		"amount":    amount,
		"direction": direction,
		"fee":       fee,
	})); err != nil {
		return nil
	}

	var pricing Pricing

	if err := pricing.SetFee(fee.Fee); err != nil {
		return nil
	}

	if direction != BUY && direction != SELL {
		errnie.Error(errnie.Err(errnie.Validation, "price: buy or sell direction required", nil))
		return nil
	}
	return PriceDecimal(pricing.Total(new(big.Rat), amount.Rat(), direction == BUY))
}
