package hawkes

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Signal measures the buy/sell trade-arrival process as

	λ(t) = μ + Σ A exp(-β(t-ti)).

It emits empirical rates before the model is identifiable, then fitted μ, λ,
A, β, spectral stability, offspring expectations, and restricted likelihood
comparisons. These are statistical measurements rather than market regimes;
forecast readiness remains false until residual and out-of-sample validation
exists.
*/
type Signal struct {
	ctx        context.Context
	cancel     context.CancelFunc
	api        *websocket.API
	trade      *Trade
	tradeCache []kraken.TradeData
}

/*
NewSignal constructs the symbol-local excitation measurement pipeline. Its
trade component is the sole owner of the mutable marked-arrival history.
*/
func NewSignal(ctx context.Context, api *websocket.API) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:    ctx,
		cancel: cancel,
		api:    api,
		trade:  NewTrade(),
	}

	signal.api.On("trade", signal.onTrade)

	return signal
}

func (signal *Signal) onTrade(data []byte) {
	if len(data) == 0 {
		return
	}

	frame := kraken.NewTrade(data)

	if len(frame.Data) == 0 {
		return
	}

	signal.tradeCache = append(signal.tradeCache, frame.Data...)
}

func (signal *Signal) Measure(
	thesis *types.Thesis,
) *types.Thesis {
	out := make([]*types.Measurement, 0, len(signal.tradeCache))

	for _, row := range signal.tradeCache {
		measurements, err := signal.trade.Measure(row)

		if err != nil {
			errnie.Error(err)
			continue
		}

		out = append(out, measurements...)
	}

	signal.tradeCache = signal.tradeCache[:0]

	thesis.Measurements = append(thesis.Measurements, out...)

	return thesis
}

func (signal *Signal) Close() error {
	signal.cancel()

	return nil
}
