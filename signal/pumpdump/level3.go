package pumpdump

import (
	"fmt"

	book "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/equation"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

func (signal *Signal) consumeLevel3(
	symbol *types.Symbol,
	level3 kraken.Level3Data,
) error {
	input, err := signal.level3Frame(symbol.Symbol, level3)

	if err != nil {
		return err
	}

	geometry, err := signal.geometry.Step(symbol.Symbol, input)

	if err != nil {
		return err
	}

	alphaChange, err := signal.depthChange.Step(
		seriesKey{symbol: symbol.Symbol, series: seriesAlphaDepth},
		sampleFrame(level3.Timestamp, input.MustGet(nmtypes.AlphaQuantity)),
	)

	if err != nil {
		return err
	}

	betaChange, err := signal.depthChange.Step(
		seriesKey{symbol: symbol.Symbol, series: seriesBetaDepth},
		sampleFrame(level3.Timestamp, input.MustGet(nmtypes.BetaQuantity)),
	)

	if err != nil {
		return err
	}

	signal.measurements.Publish(signal.bookMeasurement(
		level3.Timestamp,
		geometry,
		alphaChange,
		betaChange,
	))

	return nil
}

func (signal *Signal) level3Frame(
	symbol string,
	level3 kraken.Level3Data,
) (nmtypes.Frame, error) {
	if signal.books == nil {
		return nmtypes.Frame{}, fmt.Errorf(
			"pumpdump: authoritative Level 3 book source is required",
		)
	}

	input := eventFrame(level3.Timestamp)
	found := false
	var err error
	signal.books.Book(symbol, func(resident *book.Book) {
		if resident == nil {
			return
		}

		found = true
		err = readBook(resident, &input)
	})

	if err != nil {
		return nmtypes.Frame{}, err
	}

	if !found {
		return nmtypes.Frame{}, fmt.Errorf(
			"pumpdump: committed Level 3 book missing for %s",
			symbol,
		)
	}

	return input, nil
}

func readBook(resident *book.Book, input *nmtypes.Frame) error {
	alpha := resident.BestBid()
	beta := resident.BestAsk()

	if alpha == nil || beta == nil || alpha.Price == nil || beta.Price == nil {
		return fmt.Errorf("pumpdump: resident Level 3 book has no executable touch")
	}

	input.Put(nmtypes.AlphaPrice, alpha.Price.Float64())
	input.Put(nmtypes.BetaPrice, beta.Price.Float64())
	input.Put(nmtypes.AlphaQuantity, sideQuantity(alpha, false))
	input.Put(nmtypes.BetaQuantity, sideQuantity(beta, true))

	return nil
}

func sideQuantity(touch *book.Level, higher bool) float64 {
	quantity := 0.0

	for level := touch; level != nil; {
		if level.Quantity != nil {
			quantity += level.Quantity.Float64()
		}

		if higher {
			level = level.Higher
			continue
		}

		level = level.Lower
	}

	return quantity
}

func relativeComponents(
	primitive nmtypes.Primitive,
	change nmtypes.Frame,
) (nmtypes.Frame, error) {
	relative, found := change.Get(equation.SymbolRelativeChange)

	if !found {
		return nmtypes.Frame{}, nil
	}

	input := nmtypes.Frame{}
	input.Put(equation.SymbolChange, relative)
	_, output, err := nmtypes.Step(primitive, nmtypes.Frame{}, input)

	return output, err
}
