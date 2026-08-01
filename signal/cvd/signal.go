package cvd

import (
	"context"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	signalshared "github.com/theapemachine/symm/signal"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Signal is the Absorption perspective, measuring signed aggressor flow against
price response. Categories belong in logic; this signal emits numerical scores
only.
*/
type Signal struct {
	status        types.Status
	thesis        *types.Thesis
	ctx           context.Context
	cancel        context.CancelFunc
	api           *websocket.API
	planner       *strategy.Planner
	ui            chan []byte
	subscriptions map[string]*types.Subscription[any]
	subscribers   *sync.Map
	mu            sync.Mutex
}

/*
NewSignal creates the CVD perspective with independent rolling state for each
symbol so one market's aggressor history cannot leak into another's evidence.
*/
func NewSignal(
	ctx context.Context,
	api *websocket.API,
	planner *strategy.Planner,
	ui chan []byte,
	subscriptions map[string]*types.Subscription[any],
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		status:  types.INITIALIZING,
		ctx:     ctx,
		cancel:  cancel,
		api:     api,
		planner: planner,
		ui:      ui,
		subscriptions: subscriptions,
		subscribers: &sync.Map{},
	}

	signal.thesis.Causal.Store("signal:cvd:sample", algorithm.NewTradeFlowSample())
	signal.thesis.Causal.Store("signal:cvd:flow", equation.NewFlow())
	signal.thesis.Causal.Store("signal:cvd:midpoints", make(map[string]float64))

	signal.status = types.READY
	signal.run()
	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceCVD)
}

func (signal *Signal) Status() types.Status {
	return signal.status
}

func (signal *Signal) Subscribe(
	channel string,
	subscription *types.Subscription[any],
) *types.Subscription[any] {
	return signalshared.Subscribe(
		&signal.mu,
		signal.subscribers,
		channel,
		subscription,
	)
}

func (signal *Signal) publishThesis() {
	subscribers, ok := signal.subscribers.Load("thesis")

	if ok && subscribers != nil {
		for _, subscriber := range subscribers.([]*types.Subscription[any]) {
			subscriber.Send(signal.thesis)
		}
	}
}

func (signal *Signal) run() {
	go func() {
		for {
			select {
			case <-signal.ctx.Done():
				return
			case ticker := <-signal.subscriptions["ticker"].Channel:
				if ticker, ok := ticker.(*kraken.Ticker); ok {
					signal.onTicker(ticker)
				}
			case trade := <-signal.subscriptions["trade"].Channel:
				if trade, ok := trade.(*kraken.Trade); ok {
					signal.onTrade(trade)
				}
			}
		}
	}()
}

func (signal *Signal) onTicker(ticker *kraken.Ticker) {
	signal.thesis.AppendMeasurements(
		types.SourceCVD,
		signal.Calculate(ticker.Data, nil, nil),
		types.Stamp{At: time.Now(), Entity: types.MarketTicker},
	)

	signal.publishThesis()
}

func (signal *Signal) onTrade(trade *kraken.Trade) {
	signal.thesis.AppendMeasurements(
		types.SourceCVD,
		signal.Calculate(nil, trade.Data, nil),
		types.Stamp{At: time.Now(), Entity: types.MarketTrade},
	)

	signal.publishThesis()
}

func (signal *Signal) Calculate(
	tickers []kraken.TickerData,
	trades []kraken.TradeData,
	books []kraken.BookData,
) []*types.Measurement {
	out := make([]*types.Measurement, 0, len(trades))
	var focusMeasurements []*types.Measurement

	for _, row := range tickers {
		if row.Symbol == "" || row.Timestamp.IsZero() {
			continue
		}

		if row.Bid == nil || row.Ask == nil {
			continue
		}

		if row.Bid.Sign() <= 0 || row.Ask.Cmp(row.Bid) <= 0 {
			continue
		}

		found, _ := signal.thesis.Causal.Load("signal:cvd:midpoints")
		found.(map[string]float64)[row.Symbol] = (row.Bid.Float64() + row.Ask.Float64()) / 2
	}

	for _, row := range trades {
		if row.Symbol == "" || row.Price.Sign() <= 0 || row.Qty <= 0 {
			continue
		}

		found, _ := signal.thesis.Causal.Load("signal:cvd:midpoints")
		midpoint, exists := found.(map[string]float64)[row.Symbol]

		if !exists {
			continue
		}

		measurements, err := signal.measureTrade(row, midpoint)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"cvd: failed to measure trade",
				err,
			))
			continue
		}

		out = append(out, measurements...)

		if row.Symbol == types.Focus() {
			focusMeasurements = append(focusMeasurements, measurements...)
		}
	}

	if len(focusMeasurements) > 0 {
		utils.Publish(signal.ui, datura.NewMap("measurements", focusMeasurements))
	}

	return out
}

/*
measureTrade separates execution notional from midpoint response before
classifying one aggressor observation through the adaptive CVD window.
*/
func (signal *Signal) measureTrade(
	row kraken.TradeData,
	midpoint float64,
) ([]*types.Measurement, error) {
	found, _ := signal.thesis.Causal.Load("signal:cvd:sample")
	input, ready, err := found.(*algorithm.TradeFlowSample).Measure(algorithm.TradeFlowInput{
		Symbol:        row.Symbol,
		Price:         row.Price.Float64(),
		ResponsePrice: midpoint,
		Quantity:      row.Qty,
		Side:          row.Side,
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		))
	}

	if !ready {
		return nil, nil
	}

	found, _ = signal.thesis.Causal.Load("signal:cvd:flow")
	output, err := found.(*equation.Flow).Measure(input)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		))
	}

	return signal.cvdMeasurements(row, output), nil
}

/*
cvdMeasurements maps signed-flow output into one source×symbol row whose
Metrics map preserves each reading's unit and normalization.
*/
func (signal *Signal) cvdMeasurements(
	row kraken.TradeData,
	output equation.FlowOutput,
) []*types.Measurement {
	validity := types.MeasurementValidity{
		State:     types.ValidityValid,
		Readiness: types.ReadinessObservation,
	}
	scale := types.ScaleReference{
		Kind:    types.ScaleObservationWindow,
		From:    row.Timestamp,
		Through: row.Timestamp,
	}
	measurement := &types.Measurement{
		Source:   types.SourceCVD,
		Symbol:   row.Symbol,
		At:       row.Timestamp,
		Maturity: signal.thesis.Tick,
		Validity: validity,
		Metrics:  make(map[string]types.MetricSample, 7),
		Scale:    scale,
	}
	measurement.Metrics[types.MetricKey(types.MetricAbsorption, types.SideNone)] = types.MetricSample{Raw: output.Absorption, Normalized: types.NormalizeFinite(output.Absorption), Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricDrive, types.SideNone)] = types.MetricSample{Raw: output.Drive, Normalized: types.NormalizeFinite(output.Drive), Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricBalance, types.SideNone)] = types.MetricSample{Raw: output.Balance, Normalized: types.NormalizeFinite(output.Balance), Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricStarvation, types.SideNone)] = types.MetricSample{Raw: output.Starvation, Normalized: types.NormalizeFinite(output.Starvation), Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricStrength, types.SideNone)] = types.MetricSample{Raw: output.Value, Normalized: types.NormalizeFinite(output.Value), Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricNetFraction, types.SideNone)] = types.MetricSample{Raw: output.NetFraction, Normalized: types.NormalizeFinite(output.NetFraction), Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricNet, types.SideNone)] = types.MetricSample{Raw: output.Net, Unit: types.UnitQuoteCurrency}

	return []*types.Measurement{measurement}
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
