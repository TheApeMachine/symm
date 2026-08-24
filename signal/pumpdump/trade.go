package pumpdump

import (
	"time"

	"github.com/theapemachine/symm/kraken"
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
		signal.measurements.Publish(signal.tradeMeasurement(
			trade,
			acceleration,
			nmtypes.Frame{},
			nmtypes.Frame{},
			nmtypes.Frame{},
			nmtypes.Frame{},
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
		signal.measurements.Publish(signal.tradeMeasurement(
			trade,
			acceleration,
			rateNormalized,
			nmtypes.Frame{},
			nmtypes.Frame{},
			nmtypes.Frame{},
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

	_, polarized, err := nmtypes.Step(
		signal.polarize,
		nmtypes.Frame{},
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

	signal.measurements.Publish(signal.tradeMeasurement(
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
	rate nmtypes.Frame,
	polarized nmtypes.Frame,
) (nmtypes.Frame, error) {
	ratio, found := rate.Get(equation.SymbolRatio)

	if !found {
		return nmtypes.Frame{}, nil
	}

	rateChange, err := signal.rateChange.Step(
		seriesKey{symbol: symbol, series: seriesRateRatio},
		sampleFrame(at, ratio),
	)

	if err != nil {
		return nmtypes.Frame{}, err
	}

	relative, hasRelative := rateChange.Get(equation.SymbolRelativeChange)

	if !hasRelative {
		return nmtypes.Frame{}, nil
	}

	declineInput := nmtypes.Frame{}
	declineInput.Put(equation.SymbolChange, relative)
	_, decline, err := nmtypes.Step(
		signal.decompose,
		nmtypes.Frame{},
		declineInput,
	)

	if err != nil {
		return nmtypes.Frame{}, err
	}

	declineValue := decline.MustGet(equation.SymbolBeta)
	alpha, hasAlpha := polarized.Get(equation.SymbolAlphaNormalized)
	beta, hasBeta := polarized.Get(equation.SymbolBetaNormalized)

	if !hasAlpha || !hasBeta {
		return nmtypes.Frame{}, nil
	}

	output := nmtypes.Frame{}
	alphaExhaustion, err := product(declineValue, beta)

	if err != nil {
		return nmtypes.Frame{}, err
	}

	betaExhaustion, err := product(declineValue, alpha)

	if err != nil {
		return nmtypes.Frame{}, err
	}

	output.Put(nmtypes.AlphaQuantity, alphaExhaustion)
	output.Put(nmtypes.BetaQuantity, betaExhaustion)
	_, output, err = nmtypes.Step(signal.separate, nmtypes.Frame{}, output)

	return output, err
}

func product(left float64, right float64) (float64, error) {
	input := nmtypes.Frame{}
	input.Put(calculus.SymbolLeft, left)
	input.Put(calculus.SymbolRight, right)
	_, output, err := nmtypes.Step(calculus.Product, nmtypes.Frame{}, input)

	if err != nil {
		return 0, err
	}

	return output.MustGet(calculus.SymbolResult), nil
}
