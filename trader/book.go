package trader

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Book struct {
	signals     []types.Signal[any]
	instruments map[string]kraken.InstrumentPair
}

func NewBook(signals []types.Signal[any]) *Book {
	return &Book{
		signals:     signals,
		instruments: map[string]kraken.InstrumentPair{},
	}
}

func (book *Book) ObserveInstruments(data kraken.InstrumentData) {
	for _, pair := range data.Pairs {
		if pair.Symbol == "" || !pair.HasIncrement() {
			continue
		}

		book.instruments[pair.Symbol] = pair
	}
}

func (book *Book) Measure(message kraken.BookDataSlice) ([]*types.Measurement, error) {
	measurements := make([]*types.Measurement, 0)

	for _, msg := range message {
		row, err := book.annotate(msg)

		if err != nil {
			return nil, err
		}

		for _, signal := range book.signals {
			measurement, err := signal.Measure(row, &types.CrossSection{})

			if err != nil {
				errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					err.Error(),
					err,
				))
			}

			price := 0.0
			if len(row.Bids) > 0 && len(row.Asks) > 0 {
				price = (row.Bids[0].Price.Float64() + row.Asks[0].Price.Float64()) / 2
			}

			for _, item := range measurement {
				if item.Metrics == nil {
					item.Metrics = map[string]float64{}
				}

				if price > 0 {
					item.Metrics["price"] = price
				}
			}

			measurements = append(measurements, measurement...)
		}
	}

	return measurements, nil
}

func (book *Book) annotate(row kraken.BookData) (kraken.BookData, error) {
	pair, ok := book.instruments[row.Symbol]

	if !ok {
		return kraken.BookData{}, errnie.Err(
			errnie.Validation,
			"trader: book price increment missing for "+row.Symbol,
			nil,
		)
	}

	row.PriceIncrement = pair.Increment()

	return row, nil
}
