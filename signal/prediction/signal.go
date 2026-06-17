package prediction

import (
	"context"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
)

var featureSources = []logic.SourceType{
	logic.SourceFluid,
	logic.SourceHawkes,
	logic.SourcePumpDump,
	logic.SourceDepthFlow,
	logic.SourceSentiment,
	logic.SourceCorrelation,
	logic.SourceCausal,
	logic.SourceLeadLag,
	logic.SourceLiquidity,
	logic.SourceExhaustion,
	logic.SourceCVD,
	logic.SourceToxicity,
	logic.SourceManifold,
}

const learningTargetScaleFloor = 1e-4

type pendingForecast struct {
	matureAt      time.Time
	anchorPrice   float64
	forecast      float64
	features      []float64
	movementScale float64
	regime        predictionRegime
}

/*
Signal attempts to predict the future movement of a symbol based
on past behavior, market conditions, and other factors.
*/
type Signal struct {
	ctx              context.Context
	cancel           context.CancelFunc
	err              error
	pool             *qpool.Q[any]
	subscribers      *sync.Map
	uiBroadcast      *qpool.BroadcastGroup
	horizon          time.Duration
	forecastInterval time.Duration
	learningRate     float64
	chart            *Chart
	trade            *Trade
	states           *sync.Map
}

/*
NewSignal subscribes to trader channel and composes per-symbol forecast state.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:              ctx,
		cancel:           cancel,
		pool:             pool,
		subscribers:      &sync.Map{},
		horizon:          60 * time.Second,
		forecastInterval: 1 * time.Second,
		learningRate:     boundedClassifierAlpha(),
		trade:            NewTrade(ctx),
		states:           &sync.Map{},
	}

	return signal
}

func (signal *Signal) Update(artifact *datura.Artifact) error {
	switch datura.Peek[string](artifact, "role") {
	case "trade":
		updates := datura.As[krakenmarket.TradeUpdates](artifact)
		signal.trade.Update(updates)

		seen := make(map[string]struct{})

		for _, tradeUpdate := range updates {
			if tradeUpdate == nil || tradeUpdate.Symbol == "" {
				continue
			}

			if _, exists := seen[tradeUpdate.Symbol]; exists {
				continue
			}

			seen[tradeUpdate.Symbol] = struct{}{}
			signal.measureTradeScope(tradeUpdate.Symbol, tradeUpdate.Timestamp)
		}
	case "measurement":
		upstream := datura.As[logic.Measurement](artifact)

		if upstream.Source != "" && upstream.Source != logic.SourcePrediction {
			state := signal.ensure(upstream.Symbol)
			state.recordFeatureMeasurement(upstream)
		}

		scope := datura.Peek[string](artifact, "scope")

		if scope != "" {
			_, _ = signal.Measure(artifact)
		}
	}

	return nil
}

func (signal *Signal) Measure(in *datura.Artifact) (logic.Measurement, error) {
	scope := datura.Peek[string](in, "scope")

	if scope == "" {
		return logic.Measurement{}, nil
	}

	return signal.measureScope(scope)
}

func (signal *Signal) measureTradeScope(symbol string, at time.Time) {
	state := signal.ensure(symbol)
	state.recordTrade()

	series := signal.trade.Series(symbol)

	if len(series.Prices) < 2 {
		return
	}

	if at.IsZero() {
		at = series.At
	}

	_, _ = state.fromSeries(signal, symbol, series.Prices, series.Volumes, nil, true, at)
}

func (signal *Signal) measureScope(scope string) (logic.Measurement, error) {
	state := signal.ensure(scope)
	series := signal.trade.Series(scope)

	if len(series.Prices) < 2 {
		return logic.Measurement{}, nil
	}

	at := series.At

	if at.IsZero() {
		at = time.Now()
	}

	return state.fromSeries(signal, scope, series.Prices, series.Volumes, nil, true, at)
}

func (signal *Signal) ensure(symbol string) *symbolState {
	if symbol == "" {
		return newSymbolState()
	}

	raw, _ := signal.states.LoadOrStore(symbol, newSymbolState())

	state, ok := raw.(*symbolState)

	if !ok {
		return newSymbolState()
	}

	return state
}

func featureSourceIndex(source logic.SourceType) int {
	for index, featureSource := range featureSources {
		if featureSource == source {
			return index
		}
	}

	return -1
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() error {
	signal.cancel()

	return nil
}
