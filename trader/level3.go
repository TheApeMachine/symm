package trader

import (
	"github.com/spf13/viper"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

type Level3 struct {
	status       types.Status
	pool         *qpool.Q[any]
	signals      []types.Signal[any]
	crossSection *types.CrossSection
	sequence     uint64
	instrument   *Instrument
	level3Book   *Level3Book
	ring         *structure.SPSCRing[[]byte]
	uiHub        *ui.Hub
}

func NewLevel3(
	pool *qpool.Q[any],
	signal *Signal,
	uiHub *ui.Hub,
	instrument *Instrument,
	level3Book *Level3Book,
) *Level3 {
	return &Level3{
		status:       types.INITIALIZING,
		pool:         pool,
		signals:      signal.Level3,
		crossSection: defaultCrossSection(signal.CrossSection),
		instrument:   instrument,
		level3Book:   level3Book,
		ring: structure.NewSPSCRing[[]byte](
			viper.GetInt("signals.feed_ring_capacity"),
			false,
		),
		uiHub: uiHub,
	}
}

func (level3 *Level3) Status() types.Status {
	return level3.status
}

/*
Drain decodes every queued level3 frame, reconciles each row against the
locally reconstructed order-level book, and returns the checksum-valid
rows as ordered events carrying their resolved top-of-book price. It
performs no signal measurement, so a Chunker can merge these events with
every other stream's before any signal sees them.
*/
func (level3 *Level3) Drain() ([]types.Event, error) {
	events := make([]types.Event, 0)

	if level3.instrument.Status() != types.READY {
		return events, nil
	}

	batchSize := level3.ring.Len()

	for i := 0; i < batchSize; i++ {
		frame := level3.ring.Pop()

		if len(frame) == 0 {
			break
		}

		message := kraken.NewLevel3DataSlice(frame)

		if level3.status != types.READY && len(message) > 0 {
			level3.status = types.READY
		}

		for _, msg := range message {
			if !level3.reconcile(msg) {
				continue
			}

			bid, ask, ok := level3.level3Book.TopOfBook(msg.Symbol)

			if !ok {
				continue
			}

			level3.sequence++
			events = append(events, types.Event{
				Stream:   "level3",
				Sequence: level3.sequence,
				At:       msg.Timestamp,
				Symbol:   msg.Symbol,
				Price:    (bid + ask) / 2,
				Row:      msg,
			})
		}
	}

	return events, nil
}

/*
MeasureEvent runs one already-ordered level3 event through this feed's
signals against snapshot, the frozen cross-section a Chunker took for
the whole drain cycle this event belongs to.
*/
func (level3 *Level3) MeasureEvent(
	event types.Event, snapshot *types.CrossSection,
) ([]*types.Measurement, error) {
	msg, ok := event.Row.(kraken.Level3Data)

	if !ok {
		return nil, nil
	}

	measurements := make([]*types.Measurement, 0)

	results := measureSignals(level3.signals, func(signal types.Signal[any]) ([]*types.Measurement, error) {
		return signal.Measure(msg, snapshot)
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

	if level3.status != types.READY && len(measurements) > 0 {
		level3.status = types.READY
		errnie.Info("level3 ready")
	}

	return measurements, nil
}

/*
Measure drains and measures this feed on its own, using its own live
cross-section rather than a frozen cycle-wide snapshot. Crypto's runtime
loop uses Chunker instead; this remains for direct single-feed use.
*/
func (level3 *Level3) Measure() ([]*types.Measurement, error) {
	events, err := level3.Drain()

	if err != nil {
		return nil, err
	}

	measurements := make([]*types.Measurement, 0)

	for _, event := range events {
		result, err := level3.MeasureEvent(event, level3.crossSection)

		if err != nil {
			return nil, err
		}

		measurements = append(measurements, result...)
	}

	publishMeasurements(level3.uiHub, measurements)

	return measurements, nil
}

func (level3 *Level3) On(data []byte) {
	frame := make([]byte, len(data))
	copy(frame, data)

	if !level3.ring.Push(frame) {
		level3.status = types.ERROR
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"trader: level3 ring full",
			nil,
		))
	}
}

/*
reconcile folds row into the symbol's locally reconstructed order-level
book and validates the exchange checksum. It reports whether the book is
trustworthy; a level3 "update" message only carries the orders that
changed, so no measurement may trust top-of-book pricing until this
returns true. On a fresh checksum failure it forces Kraken to resend a
snapshot by resubscribing the symbol's level3 channel.
*/
func (level3 *Level3) reconcile(row kraken.Level3Data) bool {
	pair, err := level3.instrument.Pair(row.Symbol)

	if err != nil {
		return false
	}

	wasInvalid := level3.level3Book.Invalid(row.Symbol)
	valid := level3.level3Book.Apply(row, pair.PricePrecision, pair.QtyPrecision)

	if valid || wasInvalid {
		return valid
	}

	errnie.Error(errnie.Err(
		errnie.Conflict,
		"trader: level3 checksum failed for "+row.Symbol+", resubscribing",
		nil,
	))

	errnie.Error(level3.instrument.ResubscribeLevel3(row.Symbol))
	return false
}
