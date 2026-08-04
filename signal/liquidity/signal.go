package liquidity

import (
	"context"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/theapemachine/datura"

	"github.com/theapemachine/nomagique/statistic"
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
	ctx           context.Context
	cancel        context.CancelFunc
	api           *websocket.API
	planner       *strategy.Planner
	ui            chan []byte
	subscriptions map[string]*types.Subscription[any]
	subscribers   *sync.Map
	subscribeMu   sync.Mutex
	observations  map[string]liquidityObservation
}

type liquidityObservation struct {
	at              time.Time
	executableDepth float64
	quoteNotional   float64
	cadence         time.Duration
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
		observations:  make(map[string]liquidityObservation),
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
					measurements := signal.Measure(thesis)

					if len(measurements) > 0 {
						thesis.Measurements.Store(
							types.SourceLiquidity,
							measurements,
						)

						thesis.Readiness.Liquidity = true
						utils.Fanout(signal.subscribers, signal.Name(), thesis)
					}
				}
			}
		}
	}()
}

/*
Measure produces the Measurements for the liquidity signal.
*/
func (signal *Signal) Measure(thesis *types.Thesis) []*types.Measurement {
	tickers := thesis.MarketTickers()

	if !signal.ingest(tickers) {
		return nil
	}

	peers, freshness, cadenceReady := signal.cohort()

	if len(peers) == 0 {
		return nil
	}

	sort.Slice(peers, func(left, right int) bool {
		return peers[left].symbol < peers[right].symbol
	})
	scale := types.ScaleReference{Kind: types.ScaleObservationWindow}

	for _, peer := range peers {
		if scale.From.IsZero() || peer.observation.at.Before(scale.From) {
			scale.From = peer.observation.at
		}

		if peer.observation.at.After(scale.Through) {
			scale.Through = peer.observation.at
		}
	}

	if cadenceReady && freshness > 0 {
		scale.From = scale.Through.Add(-freshness)
	}

	measurements := make([]*types.Measurement, 0)
	out := make([]*types.Measurement, 0)

	for _, peer := range peers {
		executableDepth := peer.observation.executableDepth
		depthPeers, notionalPeers := leaveOneOutLiquidity(peer.symbol, peers)
		depthMedian, depthOK := statistic.MedianOf(depthPeers)
		peerReady := len(depthPeers) >= 2 && depthOK && depthMedian > 0
		notionalMedian, hasNotionalMedian := statistic.MedianOf(notionalPeers)
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

		if !cadenceReady {
			validity.State = types.ValidityProvisional
			validity.Reason = appendReason(validity.Reason, "cohort cadence unavailable")
		}

		relativeDepth := 0.0
		scarcity := 0.0
		median := 0.0

		if peerReady && executableDepth > 0 {
			relativeDepth = executableDepth / depthMedian
			median = depthMedian
			deficit := math.Max(0, depthMedian-executableDepth)

			if deficit > 0 {
				deviations := absoluteDeviations(depthPeers, depthMedian)
				dispersion, _ := statistic.MedianOf(deviations)
				scarcity = deficit / (deficit + dispersion)
			}
		}

		reportedNotional := peer.observation.quoteNotional
		reportedMedian := 0.0

		if hasNotionalMedian && notionalMedian > 0 {
			reportedMedian = notionalMedian
		}

		measurement := &types.Measurement{
			Source:   types.SourceLiquidity,
			Symbol:   peer.symbol,
			At:       peer.observation.at,
			Validity: validity,
			Scale:    scale,
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricExecutableTouchDepth, types.SideNone): {
					Raw:        executableDepth,
					Normalized: types.NormalizeRatio(executableDepth, median),
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

		if peer.symbol == types.Focus() {
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

type liquidityPeer struct {
	symbol      string
	observation liquidityObservation
}

func (signal *Signal) ingest(rows []kraken.TickerData) bool {
	if signal.observations == nil {
		signal.observations = make(map[string]liquidityObservation)
	}

	changed := false

	for _, row := range rows {
		symbol := strings.TrimSpace(row.Symbol)

		if symbol == "" || row.Timestamp.IsZero() {
			continue
		}

		previous, exists := signal.observations[symbol]

		if exists && !row.Timestamp.After(previous.at) {
			continue
		}

		observation := liquidityObservation{
			at:              row.Timestamp,
			executableDepth: executableDepth(row),
			quoteNotional:   quoteNotional(row),
		}

		if exists {
			observation.cadence = row.Timestamp.Sub(previous.at)
		}

		signal.observations[symbol] = observation
		changed = true
	}

	return changed
}

func (signal *Signal) cohort() ([]liquidityPeer, time.Duration, bool) {
	latest := time.Time{}
	cadences := make([]float64, 0, len(signal.observations))

	for _, observation := range signal.observations {
		if observation.at.After(latest) {
			latest = observation.at
		}

		if observation.cadence > 0 {
			cadences = append(cadences, float64(observation.cadence))
		}
	}

	medianCadence, cadenceReady := statistic.MedianOf(cadences)
	freshness := time.Duration(medianCadence)
	peers := make([]liquidityPeer, 0, len(signal.observations))

	for symbol, observation := range signal.observations {
		if cadenceReady && freshness > 0 && latest.Sub(observation.at) > freshness {
			continue
		}

		peers = append(peers, liquidityPeer{symbol: symbol, observation: observation})
	}

	return peers, freshness, cadenceReady && freshness > 0
}

func leaveOneOutLiquidity(
	symbol string,
	peers []liquidityPeer,
) ([]float64, []float64) {
	depths := make([]float64, 0, len(peers)-1)
	notionals := make([]float64, 0, len(peers)-1)

	for _, peer := range peers {
		if peer.symbol == symbol {
			continue
		}

		if peer.observation.executableDepth > 0 {
			depths = append(depths, peer.observation.executableDepth)
		}

		if peer.observation.quoteNotional > 0 {
			notionals = append(notionals, peer.observation.quoteNotional)
		}
	}

	return depths, notionals
}

func absoluteDeviations(values []float64, center float64) []float64 {
	deviations := make([]float64, 0, len(values))

	for _, value := range values {
		deviations = append(deviations, math.Abs(value-center))
	}

	return deviations
}

func executableDepth(row kraken.TickerData) float64 {
	if row.Bid == nil || row.Ask == nil || row.BidQty <= 0 || row.AskQty <= 0 {
		return 0
	}

	bid := row.Bid.Float64()
	ask := row.Ask.Float64()

	if bid <= 0 || ask <= bid || math.IsNaN(bid) || math.IsNaN(ask) ||
		math.IsInf(bid, 0) || math.IsInf(ask, 0) {
		return 0
	}

	return math.Min(row.BidQty, row.AskQty) * (bid + ask) / 2
}

func quoteNotional(row kraken.TickerData) float64 {
	price := row.Vwap

	if price <= 0 && row.Last != nil {
		price = row.Last.Float64()
	}

	if price <= 0 || row.Volume <= 0 || math.IsNaN(price) || math.IsNaN(row.Volume) ||
		math.IsInf(price, 0) || math.IsInf(row.Volume, 0) {
		return 0
	}

	return price * row.Volume
}

func appendReason(reason, addition string) string {
	if reason == "" {
		return addition
	}

	return reason + "; " + addition
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
