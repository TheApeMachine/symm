package correlation

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	signalshared "github.com/theapemachine/symm/signal"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Signal measures whether a symbol is moving with the cohort, against it, beyond
it, or without a stable relation to it. Categories belong in logic; this signal
emits numerical scores only.
*/
type Signal struct {
	status        types.Status
	thesis        *types.Thesis
	ctx           context.Context
	cancel        context.CancelFunc
	api           *websocket.API
	planner       *strategy.Planner
	section       *Section
	ui            chan []byte
	subscriptions map[string]*types.Subscription[any]
	subscribers   *sync.Map
	mu            sync.Mutex
}

/*
NewSignal creates correlation measurement state for central market cuts so
successive ticks can establish real price relationships.
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
		section: NewSection(),
		ui:      ui,
		subscriptions: subscriptions,
		subscribers: &sync.Map{},
	}

	signal.run()
	signal.status = types.READY

	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceCorrelation)
}

func (signal *Signal) Status() types.Status {
	return signal.status
}

func (signal *Signal) Subscribe(
	channel string, subscription *types.Subscription[any],
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
			}
		}
	}()
}

func (signal *Signal) onTicker(ticker *kraken.Ticker) {
	signal.thesis.AppendMeasurements(
		types.SourceCorrelation,
		signal.Calculate(ticker.Data, nil, nil),
		types.Stamp{At: time.Now(), Entity: types.MarketTicker},
	)

	signal.publishThesis()
}

func (signal *Signal) Calculate(
	tickers []kraken.TickerData,
	trades []kraken.TradeData,
	books []kraken.BookData,
) []*types.Measurement {
	if len(tickers) == 0 {
		return nil
	}

	scoresBySymbol, err := signal.section.Measure(tickers)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent, "correlation: failed to measure tickers", err,
		))

		return nil
	}

	if len(scoresBySymbol) == 0 {
		return nil
	}

	latestAtBySymbol := make(map[string]time.Time, len(tickers))

	for _, row := range tickers {
		symbol := strings.TrimSpace(row.Symbol)

		if symbol == "" || row.Timestamp.IsZero() {
			continue
		}

		if !row.Timestamp.After(latestAtBySymbol[symbol]) {
			continue
		}

		latestAtBySymbol[symbol] = row.Timestamp
	}

	out := make([]*types.Measurement, 0, len(scoresBySymbol))
	uiOut := datura.NewMap(
		"measurements", make([]*types.Measurement, 0),
	)

	validity := types.MeasurementValidity{
		State:     types.ValidityValid,
		Readiness: types.ReadinessObservation,
	}

	for symbol, scores := range scoresBySymbol {
		at := latestAtBySymbol[symbol]

		if at.IsZero() {
			at = signal.section.LastAt(symbol)
		}

		if at.IsZero() {
			continue
		}

		measurement := correlationMeasurement(symbol, at, validity, scores)
		out = append(out, measurement)

		if measurement.Symbol == types.Focus() {
			uiOut["measurements"] = append(
				uiOut["measurements"].([]*types.Measurement), measurement,
			)
		}
	}

	if len(uiOut["measurements"].([]*types.Measurement)) > 0 {
		utils.Publish(signal.ui, uiOut)
	}

	return out
}

/*
correlationMeasurement writes the nine cohort evidence metrics for one symbol.
*/
func correlationMeasurement(
	symbol string,
	at time.Time,
	validity types.MeasurementValidity,
	scores map[string]float64,
) *types.Measurement {
	measurement := &types.Measurement{
		Source:   types.SourceCorrelation,
		Symbol:   symbol,
		At:       at,
		Validity: validity,
		Metrics:  make(map[string]types.MetricSample, 9),
	}

	measurement.Metrics[types.MetricKey(types.MetricCorrelation, types.SideNone)] = types.MetricSample{Raw: scores["correlation"], Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricSigned, types.SideNone)] = types.MetricSample{Raw: scores["signed"], Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricRelativeEnergy, types.SideNone)] = types.MetricSample{Raw: scores["relativeEnergy"], Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricHerdScore, types.SideNone)] = types.MetricSample{Raw: scores["herdScore"], Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricAlphaScore, types.SideNone)] = types.MetricSample{Raw: scores["alphaScore"], Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricNoiseScore, types.SideNone)] = types.MetricSample{Raw: scores["noiseScore"], Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricStressScore, types.SideNone)] = types.MetricSample{Raw: scores["stressScore"], Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricPeakScore, types.SideNone)] = types.MetricSample{Raw: scores["peakScore"], Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricStrength, types.SideNone)] = types.MetricSample{Raw: scores["strength"], Unit: types.UnitDimensionless}

	return measurement
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
