package correlation

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/websocket"
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
		status:        types.INITIALIZING,
		ctx:           ctx,
		cancel:        cancel,
		api:           api,
		planner:       planner,
		section:       NewSection(),
		ui:            ui,
		subscriptions: subscriptions,
		subscribers:   &sync.Map{},
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
	return utils.Subscribe(
		signal.subscribers,
		channel,
		subscription,
	)
}

func (signal *Signal) run() {
	go func() {
		for {
			select {
			case <-signal.ctx.Done():
				return
			case message := <-signal.subscriptions["thesis"].Channel:
				if thesis, ok := message.(*types.Thesis); ok {
					thesis.AppendMeasurements(
						types.SourceCorrelation,
						signal.Measure(thesis),
						types.Stamp{
							At:     time.Now(),
							Entity: types.MarketTicker,
							Source: types.SourceCorrelation,
						},
					)

					utils.Fanout(signal.subscribers, signal.Name(), thesis)
				}
			}
		}
	}()
}

func (signal *Signal) Measure(thesis *types.Thesis) []*types.Measurement {
	tickers, trades, _ := thesis.Market()

	if len(tickers) == 0 && len(trades) == 0 {
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

		measurement := &types.Measurement{
			Source:   types.SourceCorrelation,
			Symbol:   symbol,
			At:       at,
			Validity: validity,
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricCorrelation, types.SideNone): {
					Raw:  scores["correlation"],
					Unit: types.UnitDimensionless,
				},
				types.MetricKey(types.MetricSigned, types.SideNone): {
					Raw:  scores["signed"],
					Unit: types.UnitDimensionless,
				},
				types.MetricKey(types.MetricRelativeEnergy, types.SideNone): {
					Raw:  scores["relativeEnergy"],
					Unit: types.UnitDimensionless,
				},
				types.MetricKey(types.MetricHerdScore, types.SideNone): {
					Raw:  scores["herdScore"],
					Unit: types.UnitDimensionless,
				},
				types.MetricKey(types.MetricAlphaScore, types.SideNone): {
					Raw:  scores["alphaScore"],
					Unit: types.UnitDimensionless,
				},
				types.MetricKey(types.MetricNoiseScore, types.SideNone): {
					Raw:  scores["noiseScore"],
					Unit: types.UnitDimensionless,
				},
				types.MetricKey(types.MetricStressScore, types.SideNone): {
					Raw:  scores["stressScore"],
					Unit: types.UnitDimensionless,
				},
				types.MetricKey(types.MetricPeakScore, types.SideNone): {
					Raw:  scores["peakScore"],
					Unit: types.UnitDimensionless,
				},
				types.MetricKey(types.MetricStrength, types.SideNone): {
					Raw:  scores["strength"],
					Unit: types.UnitDimensionless,
				},
			},
		}

		out = append(out, measurement)

		if symbol == types.Focus() {
			uiOut["measurements"] = append(
				uiOut["measurements"].([]*types.Measurement), measurement,
			)
		}
	}

	utils.Publish(signal.ui, uiOut)
	return out
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
