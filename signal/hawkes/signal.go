package hawkes

import (
	"context"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm/excitation"
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
	tradeCache *types.MarketFeed[kraken.TradeData]
	ui         chan []byte
}

/*
Publish sends one small datura frame to the UI the moment this signal has
measured its evidence, mirroring broker.Balance.Publish.
*/
func (signal *Signal) Publish(measurements []*types.Measurement) {
	filtered := types.FilterLatest(measurements)

	if len(filtered) == 0 {
		return
	}

	select {
	case signal.ui <- datura.Map[any]{
		"measurements": filtered,
	}.Marshal():
	default:
	}
}

/*
NewSignal constructs the symbol-local excitation measurement pipeline. Its
trade component is the sole owner of the mutable marked-arrival history.
*/
func NewSignal(ctx context.Context, api *websocket.API, ui chan []byte) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:    ctx,
		cancel: cancel,
		api:    api,
		trade:  NewTrade(),
		ui:     ui,
		tradeCache: types.NewMarketFeed[kraken.TradeData](
			viper.GetInt("signals.feed_timeline_capacity"),
			viper.GetInt("signals.feed_track_capacity"),
		),
	}

	signal.api.On("trade", signal.onTrade)

	return signal
}

/*
onTrade decodes trade updates and feeds executed flow so tape activity reaches
the grid.
*/
func (signal *Signal) onTrade(data []byte) {
	if len(data) == 0 {
		return
	}

	frame := kraken.NewTrade(data)

	if len(frame.Data) == 0 {
		return
	}

	for _, tradeData := range frame.Data {
		if err := signal.tradeCache.Observe(
			tradeData.Symbol,
			tradeData.Timestamp,
			tradeData,
		); err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"hawkes: trade observation failed for "+tradeData.Symbol,
				err,
			))
		}
	}
}

/*
Capture freezes the trade journal so every arrival through the planner boundary
reaches the excitation process exactly once.
*/
func (signal *Signal) Capture(at time.Time) error {
	return signal.tradeCache.Capture(at)
}

/*
Outcome returns the latest measured Hawkes outcome for one symbol.
*/
func (trade *Trade) Outcome(symbol string) (excitation.Outcome, bool) {
	if trade == nil || trade.process == nil {
		return excitation.Outcome{}, false
	}

	return trade.process.Outcome(symbol)
}

/*
Symbols returns every symbol with retained Hawkes excitation state.
*/
func (trade *Trade) Symbols() []string {
	if trade == nil || trade.process == nil {
		return nil
	}

	return trade.process.Symbols()
}

/*
Outcome returns the latest measured Hawkes outcome for one symbol.
*/
func (signal *Signal) Outcome(symbol string) (excitation.Outcome, bool) {
	if signal == nil || signal.trade == nil {
		return excitation.Outcome{}, false
	}

	return signal.trade.Outcome(symbol)
}

/*
Symbols returns every symbol with retained Hawkes excitation state.
*/
func (signal *Signal) Symbols() []string {
	if signal == nil || signal.trade == nil {
		return nil
	}

	return signal.trade.Symbols()
}

/*
Measure converts the receiver's current market input into typed measurements
so downstream logic consumes explicit evidence.
*/
func (signal *Signal) Measure(
	thesis *types.Thesis,
) *types.Thesis {
	rows, err := signal.tradeCache.Drain(thesis.At)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"hawkes: trade drain failed",
			err,
		))
		return thesis
	}

	out := make([]*types.Measurement, 0, len(rows))

	for _, row := range rows {
		measurements, err := signal.trade.Measure(row)

		if err != nil {
			errnie.Error(err)
			continue
		}

		out = append(out, measurements...)
	}

	thesis.Measurements = append(thesis.Measurements, out...)
	signal.Publish(out)

	return thesis
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()

	return nil
}
