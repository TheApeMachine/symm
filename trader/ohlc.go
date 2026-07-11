package trader

import (
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type OHLC struct {
	status       types.Status
	signals      []types.Signal[any]
	crossSection *types.CrossSection
	sequence     uint64
	ring         *structure.SPSCRing[[]byte]
	uiHub        chan []byte
}

func NewOHLC(signal *Signal, uiHub chan []byte) *OHLC {
	return &OHLC{
		status:       types.INITIALIZING,
		signals:      signal.OHLC,
		crossSection: defaultCrossSection(signal.CrossSection),
		ring: structure.NewSPSCRing[[]byte](
			viper.GetInt("signals.feed_ring_capacity"),
			true,
		),
		uiHub: uiHub,
	}
}

func (ohlc *OHLC) Status() types.Status {
	return ohlc.status
}

/*
Drain decodes every queued OHLC frame into ordered events, performing no
signal measurement, so a Chunker can merge these events with every other
stream's before any signal sees them.
*/
func (ohlc *OHLC) Drain() ([]types.Event, error) {
	events := make([]types.Event, 0)

	batchSize := ohlc.ring.Len()

	for i := 0; i < batchSize; i++ {
		frame := ohlc.ring.Pop()

		if len(frame) == 0 {
			break
		}

		message := kraken.NewOHLC(frame).Data

		if ohlc.status != types.READY && len(message) > 0 {
			ohlc.status = types.READY
		}

		for _, msg := range message {
			ohlc.sequence++
			events = append(events, types.Event{
				Stream:   "ohlc",
				Sequence: ohlc.sequence,
				At:       msg.Timestamp,
				Symbol:   msg.Symbol,
				Price:    msg.Close,
				Row:      msg,
			})
		}
	}

	return events, nil
}

/*
MeasureEvent runs one already-ordered OHLC event through this feed's
signals against snapshot, the frozen cross-section a Chunker took for
the whole drain cycle this event belongs to.
*/
func (ohlc *OHLC) MeasureEvent(
	event types.Event, snapshot *types.CrossSection,
) ([]*types.Measurement, error) {
	msg, ok := event.Row.(kraken.OHLCData)

	if !ok {
		return nil, nil
	}

	measurements := make([]*types.Measurement, 0)

	results := measureSignals(ohlc.signals, func(signal types.Signal[any]) ([]*types.Measurement, error) {
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

		if len(result.measurements) == 0 {
			continue
		}

		measurements = append(measurements, result.measurements...)
	}

	if ohlc.status != types.READY && len(measurements) > 0 {
		ohlc.status = types.READY
		errnie.Info("ohlc ready")
	}

	return measurements, nil
}

/*
Measure drains and measures this feed on its own, using its own live
cross-section rather than a frozen cycle-wide snapshot. Crypto's runtime
loop uses Chunker instead; this remains for direct single-feed use.
*/
func (ohlc *OHLC) Measure() ([]*types.Measurement, error) {
	events, err := ohlc.Drain()

	if err != nil {
		return nil, err
	}

	measurements := make([]*types.Measurement, 0)

	for _, event := range events {
		result, err := ohlc.MeasureEvent(event, ohlc.crossSection)

		if err != nil {
			return nil, err
		}

		measurements = append(measurements, result...)
	}

	select {
	case ohlc.uiHub <- datura.Map[any]{
		"measurements": measurements,
	}.Marshal():
	default:
	}

	return measurements, nil
}

func (ohlc *OHLC) On(data []byte) {
	frame := make([]byte, len(data))
	copy(frame, data)

	if !ohlc.ring.Push(frame) {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"trader: ohlc ring full",
			nil,
		))
	}
}
