package trader

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

type Ticker struct {
	pool         *qpool.Q[any]
	status       types.Status
	signals      []types.Signal[any]
	crossSection *types.CrossSection
	ring         *structure.SPSCRing[[]byte]
	uiHub        *ui.Hub
	rows         map[string]kraken.TickerData
}

func NewTicker(pool *qpool.Q[any], signal *Signal, uiHub *ui.Hub) *Ticker {
	return &Ticker{
		status:       types.INITIALIZING,
		pool:         pool,
		signals:      signal.Ticker,
		crossSection: signal.CrossSection,
		ring:         structure.NewSPSCRing[[]byte](8*1024, false),
		uiHub:        uiHub,
	}
}

func (ticker *Ticker) Status() types.Status {
	return ticker.status
}

func (ticker *Ticker) Measure() ([]*types.Measurement, error) {
	measurements := make([]*types.Measurement, 0)

	for {
		frame := ticker.ring.Pop()

		if len(frame) == 0 {
			break
		}

		message := kraken.NewTickerDataSlice(frame)

		if ticker.status != types.READY && len(message) > 0 {
			ticker.status = types.READY
		}

		if ticker.crossSection != nil {
			if err := ticker.crossSection.Observe(message); err != nil {
				return nil, errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					err.Error(),
					err,
				))
			}
		}

		for _, msg := range message {
			row, ready := ticker.apply(msg)

			if !ready {
				continue
			}

			results := measureSignals(ticker.signals, func(signal types.Signal[any]) ([]*types.Measurement, error) {
				return signal.Measure(row, ticker.crossSection)
			})

			for _, result := range results {
				if result.err != nil {
					return nil, errnie.Error(errnie.Err(
						errnie.UnprocessableContent,
						result.err.Error(),
						result.err,
					))
				}

				price := (&row.Bid).Add(&row.Ask).Div(decimal.NewFromInt64(2))

				for _, item := range result.measurements {
					if item.Metrics == nil {
						item.Metrics = map[string]float64{}
					}

					if price.Sign() > 0 {
						item.Metrics["price"] = price.Float64()
					}
				}

				measurements = append(measurements, result.measurements...)

				if ticker.uiHub != nil && len(result.measurements) > 0 {
					ticker.uiHub.Messages <- datura.Map[any]{
						"measurements": result.measurements,
					}.Marshal()
				}
			}
		}
	}

	if ticker.status != types.READY && len(measurements) > 0 {
		ticker.status = types.READY
		errnie.Info("ticker ready")
	}

	return measurements, nil
}

func (ticker *Ticker) apply(row kraken.TickerData) (kraken.TickerData, bool) {
	if row.Symbol == "" {
		return kraken.TickerData{}, false
	}

	if ticker.rows == nil {
		ticker.rows = map[string]kraken.TickerData{}
	}

	merged := ticker.rows[row.Symbol]

	if row.Bid.Float64() > 0 {
		merged.Bid = row.Bid
	}

	if row.BidQty > 0 {
		merged.BidQty = row.BidQty
	}

	if row.Ask.Float64() > 0 {
		merged.Ask = row.Ask
	}

	if row.AskQty > 0 {
		merged.AskQty = row.AskQty
	}

	if row.Last.Float64() > 0 {
		merged.Last = row.Last
	}

	if row.Volume > 0 {
		merged.Volume = row.Volume
	}

	if row.Vwap > 0 {
		merged.Vwap = row.Vwap
	}

	if row.Low.Float64() > 0 {
		merged.Low = row.Low
	}

	if row.High.Float64() > 0 {
		merged.High = row.High
	}

	if row.Change.Float64() != 0 {
		merged.Change = row.Change
	}

	if row.ChangePct != 0 {
		merged.ChangePct = row.ChangePct
	}

	if !row.Timestamp.IsZero() {
		merged.Timestamp = row.Timestamp
	}

	merged.Symbol = row.Symbol
	ticker.rows[row.Symbol] = merged

	if merged.Last.Float64() <= 0 || merged.Bid.Float64() <= 0 || merged.Ask.Float64() <= 0 || merged.Volume <= 0 {
		return merged, false
	}

	return merged, true
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
