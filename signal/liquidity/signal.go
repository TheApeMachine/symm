package liquidity

import (
	"context"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/theapemachine/datura"

	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Signal is the Scarcity perspective, identifying opportunities where current
executable touch depth is thin relative to peers. Reported-volume notional is
retained as a separate turnover context and never mixed into the book-depth score.
*/
type Signal struct {
	status        types.Status
	ctx           context.Context
	cancel        context.CancelFunc
	api           *websocket.API
	planner       *strategy.Planner
	ui            chan []byte
	subscriptions map[string]*types.Subscription[any]
	subscribers   *sync.Map
	subscribeMu   sync.Mutex
}

/*
NewSignal creates liquidity measurement state for central market cuts so each
tick can compare executable liquidity across the observed cohort.
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
		ui:            ui,
		subscriptions: subscriptions,
		subscribers:   &sync.Map{},
	}

	signal.status = types.READY
	signal.run()
	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceLiquidity)
}

func (signal *Signal) Status() types.Status {
	return signal.status
}

func (signal *Signal) Subscribe(
	channel string,
	subscription *types.Subscription[any],
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
						types.SourceLiquidity,
						signal.Measure(thesis),
						types.Stamp{
							At:     time.Now(),
							Entity: types.MarketTicker,
							Source: types.SourceLiquidity,
						},
					)

					utils.Fanout(signal.subscribers, signal.Name(), thesis)
				}
			}
		}
	}()
}

/*
Measure produces the Measurements for the liquidity signal.
*/
func (signal *Signal) Measure(thesis *types.Thesis) []*types.Measurement {
	tickers, _, _ := thesis.Market()

	if len(tickers) == 0 {
		return nil
	}

	if thesis.CrossSection == nil {
		return nil
	}

	// Retain the full observed cohort so an isolated single-symbol event still
	// reports every peer's latest executable liquidity in the same central cut.
	thesis.CrossSection.Measure(tickers)

	peers := make([]types.SymbolMetric, 0)
	notionalPeers := make([]float64, 0)
	depthPeers := make([]float64, 0)

	thesis.CrossSection.Metrics.Range(func(_, value any) bool {
		metric := value.(types.SymbolMetric)
		peers = append(peers, metric)

		if metric.QuoteNotional > 0 {
			notionalPeers = append(notionalPeers, metric.QuoteNotional)
		}

		if metric.ExecutableDepth > 0 {
			depthPeers = append(depthPeers, metric.ExecutableDepth)
		}

		return true
	})

	sort.Slice(peers, func(left, right int) bool {
		return peers[left].Symbol < peers[right].Symbol
	})
	scale := types.ScaleReference{Kind: types.ScaleObservationWindow}

	for _, peer := range peers {
		if scale.From.IsZero() || peer.At.Before(scale.From) {
			scale.From = peer.At
		}

		if peer.At.After(scale.Through) {
			scale.Through = peer.At
		}
	}

	depthMedian, depthOK := statistic.MedianOf(depthPeers)
	peerReady := len(depthPeers) >= 2 && depthOK && depthMedian > 0
	notionalMedian, hasNotionalMedian := statistic.MedianOf(notionalPeers)

	measurements := make([]*types.Measurement, 0)
	out := make([]*types.Measurement, 0)

	for _, peer := range peers {
		executableDepth := peer.ExecutableDepth
		validity := types.MeasurementValidity{
			State:     types.ValidityValid,
			Readiness: types.ReadinessObservation,
		}
		if !peerReady || executableDepth <= 0 {
			validity.State = types.ValidityProvisional

			if executableDepth <= 0 {
				validity.Reason = "executable touch depth unavailable"
			}

			if !peerReady {
				if validity.Reason != "" {
					validity.Reason += "; peer executable-depth median unavailable"
				}

				if validity.Reason == "" {
					validity.Reason = "peer executable-depth median unavailable"
				}
			}
		}

		relativeDepth := 0.0
		scarcity := 0.0
		median := 0.0

		if peerReady && executableDepth > 0 {
			relativeDepth = executableDepth / depthMedian
			scarcity = math.Max(0, 1-relativeDepth)
			median = depthMedian
		}

		reportedNotional := peer.QuoteNotional
		reportedMedian := 0.0

		if hasNotionalMedian && notionalMedian > 0 {
			reportedMedian = notionalMedian
		}

		measurement := &types.Measurement{
			Source:   types.SourceLiquidity,
			Symbol:   peer.Symbol,
			At:       peer.At,
			Validity: validity,
			Scale:    scale,
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricExecutableTouchDepth, types.SideNone): {
					Raw:        executableDepth,
					Normalized: types.NormalizeFinite(relativeDepth),
					Unit:       types.UnitQuoteCurrency,
				},
				types.MetricKey(types.MetricRelativeTouchDepth, types.SideNone): {
					Raw:        relativeDepth,
					Normalized: types.NormalizeFinite(relativeDepth),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricScarcityScore, types.SideNone): {
					Raw:        scarcity,
					Normalized: types.NormalizeFinite(scarcity),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricExecutableTouchDepthMedian, types.SideNone): {
					Raw:  median,
					Unit: types.UnitQuoteCurrency,
				},
				types.MetricKey(types.MetricReportedVolumeNotional, types.SideNone): {
					Raw:  reportedNotional,
					Unit: types.UnitQuoteCurrency,
				},
				types.MetricKey(types.MetricReportedVolumeNotionalMedian, types.SideNone): {
					Raw:  reportedMedian,
					Unit: types.UnitQuoteCurrency,
				},
			},
		}

		measurements = append(measurements, measurement)

		if peer.Symbol == types.Focus() {
			out = append(out, measurement)
		}
	}

	if len(out) > 0 {
		utils.Publish(signal.ui, datura.NewMap(
			"measurements", out,
		))
	}

	return measurements
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
