package pumpdump

import (
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/equation"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

func (signal *Signal) consumeTrade(
	symbol *types.Symbol,
	trade kraken.TradeData,
) error {
	input := eventFrame(trade.Timestamp)
	input.Put(nmtypes.Quantity, trade.Qty)
	input.Put(nmtypes.AlphaPrice, trade.Price.Float64())
	acceleration, err := signal.acceleration.Step(symbol.Symbol, input)

	if err != nil {
		return err
	}

	closed := acceleration.MustGet(equation.SymbolClosed) != 0

	if !closed {
		symbol.AppendMeasurement(signal.tradeMeasurement(
			trade,
			acceleration,
			nomagique.Frame{},
			nomagique.Frame{},
			nomagique.Frame{},
			nomagique.Frame{},
		))

		return nil
	}

	rate := acceleration.MustGet(calculus.SymbolRate)
	rateNormalized, err := signal.normalize.Step(
		seriesKey{symbol: symbol.Symbol, series: seriesRate},
		sampleFrame(trade.Timestamp, rate),
	)

	if err != nil {
		return err
	}

	change, hasChange := acceleration.Get(equation.SymbolChange)

	if !hasChange {
		symbol.AppendMeasurement(signal.tradeMeasurement(
			trade,
			acceleration,
			rateNormalized,
			nomagique.Frame{},
			nomagique.Frame{},
			nomagique.Frame{},
		))

		return nil
	}

	changeInput := eventFrame(trade.Timestamp)
	changeInput.Put(equation.SymbolChange, change)
	magnitude, err := absoluteSample(signal.absolute, changeInput)

	if err != nil {
		return err
	}

	returnNormalized, err := signal.normalize.Step(
		seriesKey{symbol: symbol.Symbol, series: seriesReturn},
		magnitude,
	)

	if err != nil {
		return err
	}

	_, polarized, err := nomagique.Step(
		signal.polarize,
		nomagique.Frame{},
		polarizationFrame(change, returnNormalized),
	)

	if err != nil {
		return err
	}

	exhaustion, err := signal.exhaustion(
		symbol.Symbol,
		trade.Timestamp,
		rateNormalized,
		polarized,
	)

	if err != nil {
		return err
	}

	symbol.AppendMeasurement(signal.tradeMeasurement(
		trade,
		acceleration,
		rateNormalized,
		returnNormalized,
		polarized,
		exhaustion,
	))

	return nil
}

func (signal *Signal) exhaustion(
	symbol string,
	at time.Time,
	rate nomagique.Frame,
	polarized nomagique.Frame,
) (nomagique.Frame, error) {
	ratio, found := rate.Get(equation.SymbolRatio)

	if !found {
		return nomagique.Frame{}, nil
	}

	rateChange, err := signal.rateChange.Step(
		seriesKey{symbol: symbol, series: seriesRateRatio},
		sampleFrame(at, ratio),
	)

	if err != nil {
		return nomagique.Frame{}, err
	}

	relative, hasRelative := rateChange.Get(equation.SymbolRelativeChange)

	if !hasRelative {
		return nomagique.Frame{}, nil
	}

	declineInput := nomagique.Frame{}
	declineInput.Put(equation.SymbolChange, relative)
	_, decline, err := nomagique.Step(
		signal.decompose,
		nomagique.Frame{},
		declineInput,
	)

	if err != nil {
		return nomagique.Frame{}, err
	}

	declineValue := decline.MustGet(equation.SymbolBeta)
	alpha, hasAlpha := polarized.Get(equation.SymbolAlphaNormalized)
	beta, hasBeta := polarized.Get(equation.SymbolBetaNormalized)

	if !hasAlpha || !hasBeta {
		return nomagique.Frame{}, nil
	}

	output := nomagique.Frame{}
	alphaExhaustion, err := product(declineValue, beta)

	if err != nil {
		return nomagique.Frame{}, err
	}

	betaExhaustion, err := product(declineValue, alpha)

	if err != nil {
		return nomagique.Frame{}, err
	}

	output.Put(nmtypes.AlphaQuantity, alphaExhaustion)
	output.Put(nmtypes.BetaQuantity, betaExhaustion)
	_, output, err = nomagique.Step(signal.separate, nomagique.Frame{}, output)

	return output, err
}

func product(left float64, right float64) (float64, error) {
	input := nomagique.Frame{}
	input.Put(calculus.SymbolLeft, left)
	input.Put(calculus.SymbolRight, right)
	_, output, err := nomagique.Step(calculus.Product, nomagique.Frame{}, input)

	if err != nil {
		return 0, err
	}

	return output.MustGet(calculus.SymbolResult), nil
}
