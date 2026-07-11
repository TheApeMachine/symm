package trader

import (
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Trade struct {
	status       types.Status
	signals      []types.Signal[any]
	crossSection *types.CrossSection
	sequence     uint64
	ring         *structure.SPSCRing[[]byte]
	uiHub        chan []byte
}

func NewTrade(signal *Signal, uiHub chan []byte) *Trade {
	return &Trade{
		status:       types.INITIALIZING,
		signals:      signal.Trade,
		crossSection: defaultCrossSection(signal.CrossSection),
		ring: structure.NewSPSCRing[[]byte](
			viper.GetInt("signals.feed_ring_capacity"),
			false,
		),
		uiHub: uiHub,
	}
}

func (trade *Trade) Status() types.Status {
	return trade.status
}

/*
Drain decodes every queued trade frame into ordered events, performing
no signal measurement, so a Chunker can merge these events with every
other stream's before any signal sees them.
*/
func (trade *Trade) Drain() ([]types.Event, error) {
	events := make([]types.Event, 0)

	batchSize := trade.ring.Len()

	for i := 0; i < batchSize; i++ {
		frame := trade.ring.Pop()

		if len(frame) == 0 {
			break
		}

		message := kraken.NewTrade(frame).Data

		if trade.status != types.READY && len(message) > 0 {
			trade.status = types.READY
		}

		for _, msg := range message {
			trade.sequence++
			events = append(events, types.Event{
				Stream:   "trade",
				Sequence: trade.sequence,
				At:       msg.Timestamp,
				Symbol:   msg.Symbol,
				Price:    msg.Price.Float64(),
				Row:      msg,
			})
		}
	}

	return events, nil
}

/*
MeasureEvent runs one already-ordered trade event through this feed's
signals against snapshot, the frozen cross-section a Chunker took for
the whole drain cycle this event belongs to.
*/
func (trade *Trade) MeasureEvent(
	event types.Event, snapshot *types.CrossSection,
) ([]*types.Measurement, error) {
	msg, ok := event.Row.(kraken.TradeData)

	if !ok {
		return nil, nil
	}

	measurements := make([]*types.Measurement, 0)

	results := measureSignals(trade.signals, func(signal types.Signal[any]) ([]*types.Measurement, error) {
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

	if trade.status != types.READY && len(measurements) > 0 {
		trade.status = types.READY
		errnie.Info("trade ready")
	}

	return measurements, nil
}

/*
Measure drains and measures this feed on its own, using its own live
cross-section rather than a frozen cycle-wide snapshot. Crypto's runtime
loop uses Chunker instead; this remains for direct single-feed use.
*/
func (trade *Trade) Measure() ([]*types.Measurement, error) {
	events, err := trade.Drain()

	if err != nil {
		return nil, err
	}

	measurements := make([]*types.Measurement, 0)

	for _, event := range events {
		result, err := trade.MeasureEvent(event, trade.crossSection)

		if err != nil {
			return nil, err
		}

		measurements = append(measurements, result...)
	}

	select {
	case trade.uiHub <- datura.Map[any]{
		"measurements": measurements,
	}.Marshal():
	default:
	}

	return measurements, nil
}

func (trade *Trade) On(data []byte) {
	frame := make([]byte, len(data))
	copy(frame, data)

	if !trade.ring.Push(frame) {
		trade.status = types.ERROR
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"trader: trade ring full",
			nil,
		))
	}
}
