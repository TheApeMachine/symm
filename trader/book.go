package trader

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Book struct {
	signals []types.Signal[kraken.BookData]
}

func NewBook(signals []types.Signal[kraken.BookData]) *Book {
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

			measurements = append(measurements, measurement...)
		}
	}

	return measurements, nil
}
