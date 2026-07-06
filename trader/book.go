package trader

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Book struct {
	signals []types.Signal[any]
}

func NewBook(signals []types.Signal[any]) *Book {
	return &Book{
		signals: signals,
	}
}

func (book *Book) Measure(message kraken.BookDataSlice) ([]*types.Measurement, error) {
	measurements := make([]*types.Measurement, 0)

	for _, msg := range message {
		for _, signal := range book.signals {
			measurement, err := signal.Measure(msg, &types.CrossSection{})

			if err != nil {
				errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					err.Error(),
					err,
				))
			}

			price := 0.0
			if len(msg.Bids) > 0 && len(msg.Asks) > 0 {
				price = (msg.Bids[0].Price + msg.Asks[0].Price) / 2
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
