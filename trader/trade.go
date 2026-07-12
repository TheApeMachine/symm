package trader

import (
	"fmt"

	"github.com/spf13/viper"
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
	rejected     uint64
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
	trade.reportRejected()

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

func (trade *Trade) reportRejected() {
	total := trade.ring.Rejected()

	if total == trade.rejected {
		return
	}

	rejected := total - trade.rejected
	trade.rejected = total
	errnie.Error(errnie.Err(
		errnie.UnprocessableContent,
		fmt.Sprintf("trader: %d trade frames rejected since previous drain", rejected),
		nil,
	))
}

/*
MeasureEvent runs one already-ordered trade event through this feed's
signals against snapshot, the frozen cross-section a Chunker took for
the whole drain cycle this event belongs to.
*/
func (trade *Trade) MeasureEvent(
	event types.Event, snapshot *types.CrossSection,
) ([]*types.Measurement, error) {
	measurements, err := NewTradeBatch(
		trade.signals,
		[]types.Event{event},
		snapshot,
	).Measure()

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		))
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

	measurements, err := NewTradeBatch(
		trade.signals,
		events,
		trade.crossSection,
	).Measure()

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		))
	}

	return measurements, nil
}

func (trade *Trade) On(data []byte) {
	frame := make([]byte, len(data))
	copy(frame, data)
	trade.ring.Push(frame)
}
