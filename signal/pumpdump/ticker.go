package pumpdump

import (
	"fmt"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/types"
)

func (signal *Signal) consumeTicker(
	symbol *types.Symbol,
	ticker kraken.TickerData,
) error {
	if ticker.Last == nil {
		return fmt.Errorf(
			"pumpdump: ticker requires a last price",
		)
	}

	if ticker.Last.Sign() < 0 {
		return fmt.Errorf(
			"pumpdump: ticker last price cannot be negative",
		)
	}

	// Kraken reports zero when a ticker row carries no last-price observation.
	// The Level 3 and trade paths remain authoritative for that symbol.
	if ticker.Last.Sign() == 0 {
		return nil
	}

	displacement, err := signal.tickerChange.Step(
		symbol.Symbol,
		sampleFrame(ticker.Timestamp, ticker.Last.Float64()),
	)

	if err != nil {
		return err
	}

	if _, found := displacement.Get(equation.SymbolChange); !found {
		symbol.AppendMeasurement(signal.tickerMeasurement(
			ticker,
			displacement,
			types.Frame{},
			types.Frame{},
			types.Frame{},
		))

		return nil
	}

	magnitude, err := absoluteSample(signal.absolute, displacement)

	if err != nil {
		return err
	}

	normalized, err := signal.normalize.Step(
		seriesKey{symbol: symbol.Symbol, series: seriesTickerDisplacement},
		magnitude,
	)

	if err != nil {
		return err
	}

	change := displacement.MustGet(equation.SymbolChange)
	_, polarized, err := nomagique.Step(
		signal.polarize,
		types.Frame{},
		polarizationFrame(change, normalized),
	)

	if err != nil {
		return err
	}

	symbol.AppendMeasurement(signal.tickerMeasurement(
		ticker,
		displacement,
		magnitude,
		normalized,
		polarized,
	))

	return nil
}
