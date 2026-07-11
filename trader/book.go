package trader

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

type Book struct {
	status       types.Status
	pool         *qpool.Q[any]
	signals      []types.Signal[any]
	crossSection *types.CrossSection
	sequence     uint64
	instrument   *Instrument
	orderBook    *OrderBook
	ring         *structure.SPSCRing[[]byte]
	uiHub        *ui.Hub
}

func NewBook(
	pool *qpool.Q[any],
	signal *Signal,
	uiHub *ui.Hub,
	instrument *Instrument,
	orderBook *OrderBook,
) *Book {
	return &Book{
		status:       types.INITIALIZING,
		pool:         pool,
		signals:      signal.Book,
		crossSection: defaultCrossSection(signal.CrossSection),
		instrument:   instrument,
		orderBook:    orderBook,
		ring: structure.NewSPSCRing[[]byte](
			viper.GetInt("signals.feed_ring_capacity"),
			false,
		),
		uiHub: uiHub,
	}
}

func (book *Book) Status() types.Status {
	return book.status
}

/*
Drain decodes every queued book frame, reconciles each row against the
locally reconstructed order book, and returns the checksum-valid rows as
ordered events carrying their resolved top-of-book price. It performs no
signal measurement, so a Chunker can merge these events with every other
stream's before any signal sees them.
*/
func (book *Book) Drain() ([]types.Event, error) {
	events := make([]types.Event, 0)

	if book.instrument.Status() != types.READY {
		return events, nil
	}

	batchSize := book.ring.Len()

	for i := 0; i < batchSize; i++ {
		frame := book.ring.Pop()

		if len(frame) == 0 {
			break
		}

		message := kraken.NewBookDataSlice(frame)

		if book.status != types.READY && len(message) > 0 {
			book.status = types.READY
		}

		for _, msg := range message {
			row, err := book.annotate(msg)

			if err != nil {
				errnie.Error(err)
				continue
			}

			if !book.reconcile(row) {
				continue
			}

			bid, ask, ok := book.orderBook.TopOfBook(row.Symbol)

			if !ok {
				continue
			}

			book.sequence++
			events = append(events, types.Event{
				Stream:   "book",
				Sequence: book.sequence,
				At:       row.Timestamp,
				Symbol:   row.Symbol,
				Price:    (&bid).Add(&ask).Div(decimal.NewFromInt64(2)).Float64(),
				Row:      row,
			})
		}
	}

	return events, nil
}

/*
MeasureEvent runs one already-ordered book event through this feed's
signals against snapshot, the frozen cross-section a Chunker took for
the whole drain cycle this event belongs to.
*/
func (book *Book) MeasureEvent(
	event types.Event, snapshot *types.CrossSection,
) ([]*types.Measurement, error) {
	row, ok := event.Row.(kraken.BookData)

	if !ok {
		return nil, nil
	}

	measurements := make([]*types.Measurement, 0)

	results := measureSignals(book.signals, func(signal types.Signal[any]) ([]*types.Measurement, error) {
		return signal.Measure(row, snapshot)
	})

	for _, result := range results {
		if result.err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				result.err.Error(),
				result.err,
			))
		}

		for _, item := range result.measurements {
			if item.Metrics == nil {
				item.Metrics = map[string]float64{}
			}

			if event.Price > 0 {
				item.Metrics["price"] = event.Price
			}
		}

		if len(result.measurements) == 0 {
			continue
		}

		measurements = append(measurements, result.measurements...)
	}

	if book.status != types.READY && len(measurements) > 0 {
		book.status = types.READY
		errnie.Info("book ready")
	}

	return measurements, nil
}

/*
Measure drains and measures this feed on its own, using its own live
cross-section rather than a frozen cycle-wide snapshot. Crypto's runtime
loop uses Chunker instead; this remains for direct single-feed use.
*/
func (book *Book) Measure() ([]*types.Measurement, error) {
	events, err := book.Drain()

	if err != nil {
		return nil, err
	}

	measurements := make([]*types.Measurement, 0)

	for _, event := range events {
		result, err := book.MeasureEvent(event, book.crossSection)

		if err != nil {
			return nil, err
		}

		measurements = append(measurements, result...)
	}

	publishMeasurements(book.uiHub, measurements)

	return measurements, nil
}

func (book *Book) On(data []byte) {
	frame := make([]byte, len(data))
	copy(frame, data)

	if !book.ring.Push(frame) {
		book.status = types.ERROR
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"trader: book ring full",
			nil,
		))
	}
}

/*
reconcile folds row into the symbol's locally reconstructed order book and
validates the exchange checksum. It reports whether the book is
trustworthy; a book channel "update" message only carries the levels that
changed, so no measurement may trust top-of-book pricing until this
returns true. On a fresh checksum failure it forces Kraken to resend a
snapshot by resubscribing the symbol's book channel.
*/
func (book *Book) reconcile(row kraken.BookData) bool {
	pair, err := book.instrument.Pair(row.Symbol)

	if err != nil {
		return false
	}

	wasInvalid := book.orderBook.Invalid(row.Symbol)
	valid := book.orderBook.Apply(row, pair.QtyPrecision)

	if valid || wasInvalid {
		return valid
	}

	errnie.Error(errnie.Err(
		errnie.Conflict,
		"trader: book checksum failed for "+row.Symbol+", resubscribing",
		nil,
	))

	errnie.Error(book.instrument.ResubscribeBook(row.Symbol))
	return false
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
