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

type OHLC struct {
	status  types.Status
	pool    *qpool.Q[any]
	signals []types.Signal[any]
	ring    *structure.SPSCRing[[]byte]
	uiHub   *ui.Hub
}

func NewOHLC(pool *qpool.Q[any], signal *Signal, uiHub *ui.Hub) *OHLC {
	return &OHLC{
		status:  types.INITIALIZING,
		pool:    pool,
		signals: signal.OHLC,
		ring:    structure.NewSPSCRing[[]byte](8*1024, false),
		uiHub:   uiHub,
	}
}

func (ohlc *OHLC) Status() types.Status {
	return ohlc.status
}

func (ohlc *OHLC) Measure() ([]*types.Measurement, error) {
	measurements := make([]*types.Measurement, 0)

	for {
		frame := ohlc.ring.Pop()

		if len(frame) == 0 {
			break
		}

		message := kraken.NewOHLCDataSlice(frame)

		if ohlc.status != types.READY && len(message) > 0 {
			ohlc.status = types.READY
		}

		for _, msg := range message {
			results := measureSignals(ohlc.signals, func(signal types.Signal[any]) ([]*types.Measurement, error) {
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

				if len(result.measurements) == 0 {
					continue
				}

				ohlc.uiHub.Messages <- datura.Map[any]{
					"measurements": result.measurements,
				}.Marshal()

				measurements = append(measurements, result.measurements...)
			}
		}
	}

	if ohlc.status != types.READY && len(measurements) > 0 {
		ohlc.status = types.READY
		errnie.Info("ohlc ready")
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
