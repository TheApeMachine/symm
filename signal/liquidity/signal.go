package liquidity

import (
	"context"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/theapemachine/datura"

	"github.com/theapemachine/nomagique/statistic"
	signalshared "github.com/theapemachine/symm/signal"
	"github.com/theapemachine/symm/kraken"
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
		status:  types.INITIALIZING,
		ctx:     ctx,
		cancel:  cancel,
		api:     api,
		planner: planner,
		ui:      ui,
		subscriptions: subscriptions,
		subscribers: &sync.Map{},
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
	return signalshared.Subscribe(
		&signal.mu,
		signal.subscribers,
		channel,
		subscription,
	)
}

func (signal *Signal) publishThesis() {
	if signal.subscribers == nil {
		return
	}

	subscribers, ok := signal.subscribers.Load("thesis")

	if ok && subscribers != nil {
		for _, subscriber := range subscribers.([]*types.Subscription[any]) {
			subscriber.Send(signal.thesis)
		}
	}
}

func (signal *Signal) run() {
	subscription := signal.subscriptions["ticker"]

	if subscription == nil {
		return
	}

	go func() {
		for {
			select {
			case <-signal.ctx.Done():
				return
			case ticker := <-subscription.Channel:
				if ticker, ok := ticker.(*kraken.Ticker); ok {
					signal.onTicker(ticker)
				}
			}
		}
	}()
}

func (signal *Signal) onTicker(ticker *kraken.Ticker) {
	signal.thesis.AppendMeasurements(
		types.SourceLiquidity,
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

	if signal.thesis == nil || signal.thesis.CrossSection == nil {
		return nil
	}

	// Retain the full observed cohort so an isolated single-symbol event still
	// reports every peer's latest executable liquidity in the same central cut.
	signal.thesis.CrossSection.Measure(tickers)

	peers := make([]types.SymbolMetric, 0)
	notionalPeers := make([]float64, 0)
	depthPeers := make([]float64, 0)

	signal.thesis.CrossSection.Metrics.Range(func(_, value any) bool {
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

	depthMedian, depthOK := statistic.MedianOf(depthPeers)
	peerReady := len(depthPeers) >= 2 && depthOK && depthMedian > 0
	notionalMedian, hasNotionalMedian := statistic.MedianOf(notionalPeers)

	out := make([]*types.Measurement, 0, len(peers))
	uiOut := datura.NewMap(
		"measurements", make([]*types.Measurement, 0),
	)

	for _, peer := range peers {
		executableDepth := peer.ExecutableDepth
		validity := types.MeasurementValidity{
			State:     types.ValidityValid,
			Readiness: types.ReadinessObservation,
		}
		scale := types.ScaleReference{
			Kind:    types.ScaleObservationWindow,
			From:    peer.At,
			Through: peer.At,
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
			Maturity: signal.thesis.Tick,
			Validity: validity,
			Scale:    scale,
			Metrics:  make(map[string]types.MetricSample, 6),
		}

		measurement.Metrics[types.MetricKey(types.MetricExecutableTouchDepth, types.SideNone)] = types.MetricSample{
			Raw:        executableDepth,
			Normalized: types.NormalizeFinite(relativeDepth),
			Unit:       types.UnitQuoteCurrency,
		}
		measurement.Metrics[types.MetricKey(types.MetricRelativeTouchDepth, types.SideNone)] = types.MetricSample{
			Raw:        relativeDepth,
			Normalized: types.NormalizeFinite(relativeDepth),
			Unit:       types.UnitDimensionless,
		}
		measurement.Metrics[types.MetricKey(types.MetricScarcityScore, types.SideNone)] = types.MetricSample{
			Raw:        scarcity,
			Normalized: types.NormalizeFinite(scarcity),
			Unit:       types.UnitDimensionless,
		}
		measurement.Metrics[types.MetricKey(types.MetricExecutableTouchDepthMedian, types.SideNone)] = types.MetricSample{
			Raw:  median,
			Unit: types.UnitQuoteCurrency,
		}
		measurement.Metrics[types.MetricKey(types.MetricReportedVolumeNotional, types.SideNone)] = types.MetricSample{
			Raw:  reportedNotional,
			Unit: types.UnitQuoteCurrency,
		}
		measurement.Metrics[types.MetricKey(types.MetricReportedVolumeNotionalMedian, types.SideNone)] = types.MetricSample{
			Raw:  reportedMedian,
			Unit: types.UnitQuoteCurrency,
		}

		out = append(out, measurement)

		if peer.Symbol == types.Focus() {
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
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
