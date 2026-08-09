package broker

import (
	"fmt"

	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/* Fee returns the taker fee for a symbol. */
func (price *Price) Fee(symbol string) *kraken.TradeVolumeFee {
	found, ok := price.fees.Load(price.api.Normalizer().Name(symbol))

	if !ok {
		errnie.Error(errnie.Err(
			errnie.NotFound,
			"fee not found for "+symbol,
			nil,
		))

		return nil
	}

	fee, ok := found.(kraken.TradeVolumeFee)

	if !ok {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"invalid fee type for "+symbol,
			nil,
		))

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

	resolved := make(map[string]kraken.TradeVolumeFee, len(symbols))

	for _, symbol := range symbols {
		fee, err := price.resolveFee(symbol, tradeVolumeResult.Fees)

		if err != nil {
			return err
		}

		resolved[price.normalizer.Name(symbol)] = fee
	}

	for symbol, fee := range resolved {
		price.fees.Store(symbol, fee)
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

	if err := errnie.Error(errnie.Require(map[string]any{
		"symbol":    symbol,
		"amount":    amount,
		"direction": direction,
		"fee":       fee,
	})); err != nil {
		return nil
	}

	feeAmount := decimal.ExactMul(amount, decimal.ExactDiv(
		fee.Fee, decimal.NewFromInt64(100),
	))
	amount = amount.SetScale(max(amount.GetScale(), feeAmount.GetScale()))

	switch direction {
	case BUY:
		amount = amount.Add(feeAmount)
	case SELL:
		amount = amount.Sub(feeAmount)
	}

	return amount
}
