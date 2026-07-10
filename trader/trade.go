package trader

import (
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

type Trade struct {
	status  types.Status
	pool    *qpool.Q[any]
	signals []types.Signal[any]
	ring    *structure.SPSCRing[[]byte]
	uiHub   *ui.Hub
}

func NewTrade(pool *qpool.Q[any], signal *Signal, uiHub *ui.Hub) *Trade {
	return &Trade{
		status:  types.INITIALIZING,
		pool:    pool,
		signals: signal.Trade,
		ring:    structure.NewSPSCRing[[]byte](8*1024, false),
		uiHub:   uiHub,
	}
}

func (trade *Trade) Status() types.Status {
	return trade.status
}

func (trade *Trade) Measure() ([]*types.Measurement, error) {
	measurements := make([]*types.Measurement, 0)

	for {
		frame := trade.ring.Pop()

		if len(frame) == 0 {
			break
		}

		message := kraken.NewTradeDataSlice(frame)

		if trade.status != types.READY && len(message) > 0 {
			trade.status = types.READY
		}

		for _, msg := range message {
			results := measureSignals(trade.signals, func(signal types.Signal[any]) ([]*types.Measurement, error) {
				return signal.Measure(msg, &types.CrossSection{})
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

					if msg.Price.Sign() > 0 {
						item.Metrics["price"] = msg.Price.Float64()
					}
				}

				if len(result.measurements) == 0 {
					continue
				}

				trade.uiHub.Messages <- datura.Map[any]{
					"measurements": result.measurements,
				}.Marshal()

				measurements = append(measurements, result.measurements...)
			}
		}
	}

	if trade.status != types.READY && len(measurements) > 0 {
		trade.status = types.READY
		errnie.Info("trade ready")
	}

	return measurements, nil
}

func (trade *Trade) On(data []byte) {
	frame := make([]byte, len(data))
	copy(frame, data)

	if !trade.ring.Push(frame) {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"trader: trade ring full",
			nil,
		))
	}
}
