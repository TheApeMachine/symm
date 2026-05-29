package trader

import (
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
)

func configuredExecutionCancelUrgency() float64 {
	if config.System.ExitUrgencyThreshold > 0 {
		return config.System.ExitUrgencyThreshold
	}

	return 1
}

func configuredExecutionFallbackTicks() int {
	if config.System.ExecutionMakerFallbackTicks > 0 {
		return config.System.ExecutionMakerFallbackTicks
	}

	return config.System.MinConfidenceHistory
}

func (crypto *Crypto) submitLiveEntry(
	prediction engine.Prediction,
	lead engine.Measurement,
	slot float64,
	predictedReturn float64,
	stopPrice float64,
	stopLimit float64,
	takeProfitPrice float64,
) error {
	if config.System.UseMakerEntries {
		return crypto.submitLiveMakerEntry(
			prediction, lead, slot, predictedReturn, stopPrice, stopLimit, takeProfitPrice,
		)
	}

	buy := broker.Buy{
		Symbol:         lead.Pairs[0].Wsname,
		Notional:       slot,
		Quote:          broker.Quote{Last: lead.Last, Bid: lead.Bid, Ask: lead.Ask},
		StopPrice:      stopPrice,
		LimitBelowStop: stopLimit,
	}

	if err := buy.SubmitLive(crypto.orderRouter(), crypto.wallet); err != nil {
		return err
	}

	return crypto.trackSubmittedLiveEntry(
		buy.Symbol, buy.ClOrdID, prediction, lead, slot,
		predictedReturn, stopPrice, stopLimit, takeProfitPrice,
	)
}

func (crypto *Crypto) submitLiveMakerEntry(
	prediction engine.Prediction,
	lead engine.Measurement,
	slot float64,
	predictedReturn float64,
	stopPrice float64,
	stopLimit float64,
	takeProfitPrice float64,
) error {
	limitPrice := lead.Bid

	if limitPrice <= 0 {
		limitPrice = lead.AnchorPrice()
	}

	maker := broker.Maker{
		Symbol:           lead.Pairs[0].Wsname,
		LimitPrice:       limitPrice,
		Notional:         slot,
		HasPriceDecimals: lead.Pairs[0].PairDecimals > 0,
		PriceDecimals:    lead.Pairs[0].PairDecimals,
	}

	if err := maker.SubmitLive(crypto.orderRouter(), crypto.wallet); err != nil {
		return err
	}

	return crypto.trackSubmittedLiveEntry(
		maker.Symbol, maker.ClOrdID, prediction, lead, slot,
		predictedReturn, stopPrice, stopLimit, takeProfitPrice,
	)
}

func (crypto *Crypto) trackSubmittedLiveEntry(
	symbol string,
	clOrdID string,
	prediction engine.Prediction,
	lead engine.Measurement,
	slot float64,
	predictedReturn float64,
	stopPrice float64,
	stopLimit float64,
	takeProfitPrice float64,
) error {
	// Track for runway-expiry exit (paper appends this at fill; live tracks at
	// submission). settlePredictions only flattens a symbol it actually holds,
	// so an unfilled submission is harmlessly pruned when its runway elapses.
	crypto.predictions = append(crypto.predictions, &prediction)

	return crypto.execution.Track(liveEntryOrder{
		Symbol:          symbol,
		ClOrdID:         clOrdID,
		Notional:        slot,
		Reserved:        slot,
		EntryConfidence: lead.Confidence,
		Prediction:      prediction,
		PredictedReturn: predictedReturn,
		StopPrice:       stopPrice,
		StopLimit:       stopLimit,
		TakeProfit:      takeProfitPrice,
		HasLotDecimals:  lead.Pairs[0].LotDecimals > 0,
		LotDecimals:     lead.Pairs[0].LotDecimals,
		CreatedAt:       time.Now(),
	})
}

func (crypto *Crypto) orderRouter() *broker.Router {
	return broker.NewRouter(func(value any) {
		crypto.broadcasts["orders"].Send(&qpool.QValue[any]{Value: value})
	})
}
