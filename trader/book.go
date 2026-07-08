package trader

import (
	"sync/atomic"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Book struct {
	signals     []types.Signal[any]
	instruments atomic.Value
}

func NewBook(signals []types.Signal[any]) *Book {
	book := &Book{
		signals:     signals,
	}

	book.instruments.Store(make(map[string]kraken.InstrumentPair))

	return book
}

func (book *Book) ObserveInstruments(data kraken.InstrumentData) {
	oldMap, ok := book.instruments.Load().(map[string]kraken.InstrumentPair)

	if !ok {
		oldMap = make(map[string]kraken.InstrumentPair)
	}

	newMap := make(map[string]kraken.InstrumentPair, len(oldMap)+len(data.Pairs))

	for k, v := range oldMap {
		newMap[k] = v
	}

	for _, pair := range data.Pairs {
		if pair.Symbol == "" || !pair.HasIncrement() {
			continue
		}

		newMap[pair.Symbol] = pair
	}

	book.instruments.Store(newMap)
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
	insts, ok := book.instruments.Load().(map[string]kraken.InstrumentPair)

	if !ok {
		return kraken.BookData{}, errnie.Err(
			errnie.Validation,
			"trader: book price increment missing for "+row.Symbol,
			nil,
		)
	}

	pair, ok := insts[row.Symbol]

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

/*
Instrument returns the instrument metadata for the symbol if it is cached.
*/
func (book *Book) Instrument(symbol string) (kraken.InstrumentPair, bool) {
	insts, ok := book.instruments.Load().(map[string]kraken.InstrumentPair)

	if !ok {
		return kraken.InstrumentPair{}, false
	}

	pair, ok := insts[symbol]

	return pair, ok
}
