package derivatives

import (
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/types"
)

func (signal *Signal) consumeTrade(
	symbol *types.Symbol,
	trade kraken.FuturesTradeData,
	data *DerivativesData,
) error {
	notional := trade.Price.Float64() * trade.Qty
	buyVol := 0.0
	sellVol := 0.0

	if trade.Side == "buy" {
		buyVol = notional
	}

	if trade.Side == "sell" {
		sellVol = notional
	}

	totalVol := buyVol + sellVol

	if totalVol > 0 {
		data.AggressorImbalance = (buyVol - sellVol) / totalVol
	}

	flowInput := eventFrame(trade.Timestamp)
	flowInput.Put(calculus.SymbolLeft, buyVol)
	flowInput.Put(calculus.SymbolRight, sellVol)

	flowOutput, err := signal.flow.Step(symbol.Symbol, flowInput)

	if err != nil {
		return err
	}

	data.CVD, _ = flowOutput.Get(SymbolCVD)
	flowZScore, _ := flowOutput.Get(statistic.SymbolZScore)

	if trade.Type == "liquidation" {
		if trade.Side == "buy" {
			data.LiquidationBuy += notional
		}

		if trade.Side == "sell" {
			data.LiquidationSell += notional
		}

		if totalVol > 0 && notional > 0 {
			liqInput := eventFrame(trade.Timestamp)
			liqInput.Put(calculus.SymbolLeft, notional)
			liqInput.Put(calculus.SymbolRight, totalVol)

			liqOutput, err := signal.liquidations.Step(symbol.Symbol, liqInput)

			if err != nil {
				return err
			}

			data.LiquidationIntensity, _ = liqOutput.Get(SymbolLiqIntensity)
		}
	}

	if trade.Price.Float64() <= 0 {
		return nil
	}

	priceOutput, err := signal.tradePrice.Step(
		symbol.Symbol,
		sampleFrame(trade.Timestamp, trade.Price.Float64()),
	)

	if err != nil {
		return err
	}

	priceVelocity, _ := priceOutput.Get(equation.SymbolRelativeChange)
	data.SampleCount, _ = priceOutput.Get(nmtypes.SampleCount)

	ign, sqz, build, delev, decoup := evaluateRegimes(
		priceVelocity,
		data.OIVelocity,
		data.AggressorImbalance,
		flowZScore,
		data.LiquidationBuy,
		data.LiquidationSell,
		data.LiquidationIntensity,
		data.Basis,
		data.TripartiteDivergence,
	)

	data.LeveragedIgnition = ign
	data.ShortSqueeze = sqz
	data.AdverseLeverageBuildup = build
	data.LongDeleveraging = delev
	data.DerivativesDecoupling = decoup

	return nil
}
