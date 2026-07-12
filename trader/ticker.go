package trader

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Ticker struct {
	status       types.Status
	signals      []types.Signal[any]
	crossSection *types.CrossSection
	ring         *structure.SPSCRing[[]byte]
	uiHub        chan []byte
}

func NewTicker(signal *Signal, uiHub chan []byte) *Ticker {
	return &Ticker{
		status:       types.INITIALIZING,
		signals:      signal.Ticker,
		crossSection: signal.CrossSection,
		ring: structure.NewSPSCRing[[]byte](
			viper.GetInt("signals.feed_ring_capacity"),
			true,
		),
		uiHub: uiHub,
	}
}

func (ticker *Ticker) Status() types.Status {
	return ticker.status
}

func (ticker *Ticker) Measure() ([]*types.Measurement, error) {
	measurements := make([]*types.Measurement, 0)
	var observeErr error
	batchSize := ticker.ring.Len()

	for range batchSize {
		frame := ticker.ring.Pop()

		if observeErr != nil || len(frame) == 0 {
			break
		}

		message := kraken.NewTicker(frame).Data

		if ticker.status != types.READY && len(message) > 0 {
			ticker.status = types.READY
		}

		if ticker.crossSection != nil {
			if err := ticker.crossSection.Observe(message); err != nil {
				observeErr = err
				break
			}
		}

		for _, row := range message {
			results := measureSignals(ticker.signals, func(signal types.Signal[any]) ([]*types.Measurement, error) {
				return signal.Measure(row, ticker.crossSection)
			})

			for _, result := range results {
				if result.err != nil {
					observeErr = result.err
					break
				}

				for _, item := range result.measurements {
					if item.Metrics == nil {
						item.Metrics = map[string]float64{}
					}

					if row.Bid != nil && row.Ask != nil {
						price := row.Bid.Copy().Add(row.Ask.Copy()).Div(decimal.NewFromInt64(2))

						if price.Sign() > 0 {
							item.Metrics["price"] = price.Float64()
						}
					}
				}

				if len(result.measurements) == 0 {
					continue
				}

				measurements = append(measurements, result.measurements...)
			}

			if observeErr != nil {
				break
			}
		}
	}

	if observeErr != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			observeErr.Error(),
			observeErr,
		))
	}

	if ticker.status != types.READY && len(measurements) > 0 {
		ticker.status = types.READY
		errnie.Info("ticker ready")
	}

	return measurements, nil
}

func (ticker *Ticker) On(data []byte) {
	frame := make([]byte, len(data))
	copy(frame, data)

	if !ticker.ring.Push(frame) {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"trader: ticker ring full",
			nil,
		))
	}
}
