package trader

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Book struct {
	status     types.Status
	signals    []types.Signal[any]
	instrument *Instrument
	ring       *structure.SPSCRing[[]byte]
	uiHub      chan []byte
}

func NewBook(
	signal *Signal,
	uiHub chan []byte,
	instrument *Instrument,
) *Book {
	return &Book{
		status:     types.INITIALIZING,
		signals:    signal.Book,
		instrument: instrument,
		ring:       structure.NewSPSCRing[[]byte](8*1024, true),
		uiHub:      uiHub,
	}
}

func (book *Book) Status() types.Status {
	return book.status
}

func (book *Book) Measure() ([]*types.Measurement, error) {
	measurements := make([]*types.Measurement, 0)

	if book.instrument.Status() != types.READY {
		return measurements, nil
	}

	batchSize := book.ring.Len()

	for range batchSize {
		frame := book.ring.Pop()

		if len(frame) == 0 {
			break
		}

		message := kraken.NewBook(frame).Data

		if book.status != types.READY && len(message) > 0 {
			book.status = types.READY
		}

		for _, msg := range message {
			row, err := book.annotate(msg)

			if err != nil {
				return nil, err
			}

			results := measureSignals(book.signals, func(signal types.Signal[any]) ([]*types.Measurement, error) {
				return signal.Measure(row, &types.CrossSection{})
			})

			for _, result := range results {
				if result.err != nil {
					return nil, errnie.Error(errnie.Err(
						errnie.UnprocessableContent,
						result.err.Error(),
						result.err,
					))
				}

				if len(row.Bids) == 0 || len(row.Asks) == 0 {
					continue
				}

				price := (&row.Bids[0].Price).Add(&row.Asks[0].Price).Div(decimal.NewFromInt64(2))

				for _, item := range result.measurements {
					if item.Metrics == nil {
						item.Metrics = map[string]float64{}
					}

					if price.Sign() > 0 {
						item.Metrics["price"] = price.Float64()
					}
				}

				if len(result.measurements) == 0 {
					continue
				}

				measurements = append(measurements, result.measurements...)
			}
		}
	}

	if book.status != types.READY && len(measurements) > 0 {
		book.status = types.READY
		errnie.Info("book ready")
	}

	return measurements, nil
}

func (book *Book) On(data []byte) {
	frame := make([]byte, len(data))
	copy(frame, data)

	if !book.ring.Push(frame) {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"trader: book ring full",
			nil,
		))
	}
}

func (book *Book) annotate(row kraken.BookData) (kraken.BookData, error) {
	pair, err := book.instrument.Pair(row.Symbol)

	if err != nil {
		return kraken.BookData{}, err
	}

	if !pair.HasIncrement() {
		return kraken.BookData{}, errnie.Err(
			errnie.Validation,
			"trader: book price increment missing for "+row.Symbol,
			nil,
		)
	}

	row.PriceIncrement = pair.Increment()

	return row, nil
}
