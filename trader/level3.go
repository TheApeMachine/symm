package trader

import (
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Level3 struct {
	status  types.Status
	signals []types.Signal[any]
	ring    *structure.SPSCRing[[]byte]
	uiHub   chan []byte
}

func NewLevel3(
	signal *Signal,
	uiHub chan []byte,
) *Level3 {
	return &Level3{
		status:  types.INITIALIZING,
		signals: signal.Level3,
		ring:    structure.NewSPSCRing[[]byte](8*1024, false),
		uiHub:   uiHub,
	}
}

func (level3 *Level3) Status() types.Status {
	return level3.status
}

func (level3 *Level3) Measure() ([]*types.Measurement, error) {
	measurements := make([]*types.Measurement, 0)

	for {
		frame := level3.ring.Pop()

		if len(frame) == 0 {
			break
		}

		message := kraken.NewLevel3(frame).Data

		if level3.status != types.READY && len(message) > 0 {
			level3.status = types.READY
		}

		for _, msg := range message {
			results := measureSignals(level3.signals, func(signal types.Signal[any]) ([]*types.Measurement, error) {
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

				price := 0.0

				if len(msg.Bids) > 0 && len(msg.Asks) > 0 {
					price = (msg.Bids[0].LimitPrice + msg.Asks[0].LimitPrice) / 2
				}

				for _, item := range result.measurements {
					if item.Metrics == nil {
						item.Metrics = map[string]float64{}
					}

					if price > 0 {
						item.Metrics["price"] = price
					}
				}

				if len(result.measurements) == 0 {
					continue
				}

				select {
				case level3.uiHub <- datura.Map[any]{
					"measurements": result.measurements,
				}.Marshal():
				default:
				}

				measurements = append(measurements, result.measurements...)
			}
		}
	}

	if level3.status != types.READY && len(measurements) > 0 {
		level3.status = types.READY
		errnie.Info("level3 ready")
	}

	return measurements, nil
}

func (level3 *Level3) On(data []byte) {
	frame := make([]byte, len(data))
	copy(frame, data)

	if !level3.ring.Push(frame) {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"trader: level3 ring full",
			nil,
		))
	}
}
