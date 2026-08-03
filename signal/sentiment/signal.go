package sentiment

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
Signal measures global market conviction from breadth and leadership
performance. Categories belong in logic; this signal emits numerical scores only.
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
	observations  map[string]returnObservation
}

type returnObservation struct {
	at      time.Time
	price   float64
	change  float64
	cadence time.Duration
	ready   bool
}

/*
NewSignal creates sentiment measurement state for central market cuts so every
tick can compare breadth with current leadership.
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
		observations:  make(map[string]returnObservation),
	}

	signal.status = types.READY
	signal.run()
	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceSentiment)
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
						types.SourceSentiment,
						signal.Measure(thesis),
						types.Stamp{
							At:     time.Now(),
							Entity: types.MarketTicker,
							Source: types.SourceSentiment,
						},
					)

					utils.Fanout(signal.subscribers, signal.Name(), thesis)
				}
			}
		}
	}()
}

/*
Measure produces the Measurements for the sentiment signal.
*/
func (signal *Signal) Measure(thesis *types.Thesis) []*types.Measurement {
	tickers, _, _ := thesis.Market()

	if !signal.ingest(tickers) {
		return nil
	}

	peers, freshness, cadenceReady := signal.cohort()

	if len(peers) == 0 {
		return nil
	}

	statistics := sentimentStatistics(peers)

	validity := types.MeasurementValidity{
		State:     types.ValidityValid,
		Readiness: types.ReadinessObservation,
	}

	if len(peers) < 2 {
		validity.State = types.ValidityProvisional
		validity.Reason = "peer return cohort unavailable"
	}

	if !cadenceReady {
		validity.State = types.ValidityProvisional
		validity.Reason = appendReason(validity.Reason, "cohort cadence unavailable")
	}

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

	measurements := make([]*types.Measurement, 0, len(peers))
	out := make([]*types.Measurement, 0)

	for _, peer := range peers {
		change := peer.observation.change
		leaderStrength := 0.0
		leaderEvidence := 0.0
		relativeLead := 0.0
		peerDivergenceScore := 0.0
		isLeader := statistics.leader == peer.symbol && statistics.leaderMagnitude > 0

		if isLeader {
			leaderStrength = statistics.leaderMagnitude
			leaderEvidence = statistics.leaderEvidence
			relativeLead = statistics.relativeLead
			peerDivergenceScore = statistics.divergence
		}

		strength := math.Max(
			statistics.surge,
			math.Max(peerDivergenceScore, statistics.slump),
		)

		measurement := &types.Measurement{
			Source:   types.SourceSentiment,
			Symbol:   peer.symbol,
			At:       peer.observation.at,
			Validity: validity,
			Scale:    scale,
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricChange, types.SideNone): {
					Raw:        change,
					Normalized: types.NormalizeFinite(change),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricBreadth, types.SideNone): {
					Raw:        statistics.breadth,
					Normalized: types.NormalizeSigned(statistics.breadth),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricLeaderStrength, types.SideNone): {
					Raw:        leaderStrength,
					Normalized: types.NormalizeFinite(leaderStrength),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricLeaderEvidence, types.SideNone): {
					Raw:        leaderEvidence,
					Normalized: types.NormalizeFinite(leaderEvidence),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricRelativeLead, types.SideNone): {
					Raw:        relativeLead,
					Normalized: types.NormalizeFinite(relativeLead),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricSurgeScore, types.SideNone): {
					Raw:        statistics.surge,
					Normalized: types.NormalizeFinite(statistics.surge),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricDivergentScore, types.SideNone): {
					Raw:        peerDivergenceScore,
					Normalized: types.NormalizeFinite(peerDivergenceScore),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricSlumpScore, types.SideNone): {
					Raw:        statistics.slump,
					Normalized: types.NormalizeFinite(statistics.slump),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricStrength, types.SideNone): {
					Raw:        strength,
					Normalized: types.NormalizeFinite(strength),
					Unit:       types.UnitDimensionless,
				},
			},
		}

		measurements = append(measurements, measurement)

		if measurement.Symbol == types.Focus() {
			out = append(out, measurement)
		}
	}

	if len(out) > 0 {
		utils.Publish(signal.ui, datura.NewMap("measurements", out))
	}

	return measurements
}

type sentimentPeer struct {
	symbol      string
	observation returnObservation
}

type sentimentSummary struct {
	leader          string
	leaderMagnitude float64
	leaderEvidence  float64
	relativeLead    float64
	breadth         float64
	surge           float64
	slump           float64
	divergence      float64
}

func (signal *Signal) ingest(rows []kraken.TickerData) bool {
	if signal.observations == nil {
		signal.observations = make(map[string]returnObservation)
	}

	changed := false

	for _, row := range rows {
		symbol := strings.TrimSpace(row.Symbol)

		if symbol == "" || row.Timestamp.IsZero() || row.Last == nil {
			continue
		}

		price := row.Last.Float64()

		if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
			continue
		}

		previous, exists := signal.observations[symbol]

		if exists && !row.Timestamp.After(previous.at) {
			continue
		}

		observation := returnObservation{at: row.Timestamp, price: price}

		if exists {
			observation.change = math.Log(price / previous.price)
			observation.cadence = row.Timestamp.Sub(previous.at)
			observation.ready = true
		}

		signal.observations[symbol] = observation
		changed = true
	}

	return changed
}

func (signal *Signal) cohort() ([]sentimentPeer, time.Duration, bool) {
	latest := time.Time{}
	cadences := make([]float64, 0, len(signal.observations))

	for _, observation := range signal.observations {
		if !observation.ready {
			continue
		}

		if observation.at.After(latest) {
			latest = observation.at
		}

		if observation.cadence > 0 {
			cadences = append(cadences, float64(observation.cadence))
		}
	}

	medianCadence, cadenceReady := statistic.MedianOf(cadences)
	freshness := time.Duration(medianCadence)
	peers := make([]sentimentPeer, 0, len(signal.observations))

	for symbol, observation := range signal.observations {
		if !observation.ready {
			continue
		}

		if cadenceReady && freshness > 0 && latest.Sub(observation.at) > freshness {
			continue
		}

		peers = append(peers, sentimentPeer{symbol: symbol, observation: observation})
	}

	sort.Slice(peers, func(left, right int) bool {
		return peers[left].symbol < peers[right].symbol
	})

	return peers, freshness, cadenceReady && freshness > 0
}

func sentimentStatistics(peers []sentimentPeer) sentimentSummary {
	summary := sentimentSummary{}
	changes := make([]float64, 0, len(peers))
	magnitudes := make([]float64, 0, len(peers))
	advances := 0
	declines := 0
	totalMagnitude := 0.0

	for _, peer := range peers {
		change := peer.observation.change
		magnitude := math.Abs(change)
		changes = append(changes, change)
		magnitudes = append(magnitudes, magnitude)
		totalMagnitude += magnitude

		if change > 0 {
			advances++
		}

		if change < 0 {
			declines++
		}

		if magnitude > summary.leaderMagnitude {
			summary.leader = peer.symbol
			summary.leaderMagnitude = magnitude
		}
	}

	if len(peers) == 0 {
		return summary
	}

	summary.breadth = float64(advances-declines) / float64(len(peers))
	medianChange, hasMedianChange := statistic.MedianOf(changes)
	medianMagnitude, hasMedianMagnitude := statistic.MedianOf(magnitudes)

	if hasMedianChange && hasMedianMagnitude && medianMagnitude > 0 {
		agreement := float64(max(advances, declines)) / float64(len(peers))
		summary.surge = math.Max(0, medianChange) * agreement / medianMagnitude
		summary.slump = math.Max(0, -medianChange) * agreement / medianMagnitude
	}

	if summary.leader == "" || totalMagnitude <= 0 {
		return summary
	}

	summary.relativeLead = summary.leaderMagnitude / totalMagnitude
	peerMagnitudes := make([]float64, 0, len(peers)-1)
	nonconfirming := 0
	leaderChange := 0.0

	for _, peer := range peers {
		if peer.symbol == summary.leader {
			leaderChange = peer.observation.change

			break
		}
	}

	for _, peer := range peers {
		if peer.symbol == summary.leader {
			continue
		}

		peerMagnitudes = append(peerMagnitudes, math.Abs(peer.observation.change))

		if leaderChange != 0 && leaderChange*peer.observation.change <= 0 {
			nonconfirming++
		}
	}

	peerMedian, peerMedianOK := statistic.MedianOf(peerMagnitudes)

	if !peerMedianOK || summary.leaderMagnitude <= peerMedian {
		return summary
	}

	deviations := make([]float64, 0, len(peerMagnitudes))

	for _, magnitude := range peerMagnitudes {
		deviations = append(deviations, math.Abs(magnitude-peerMedian))
	}

	peerDispersion, _ := statistic.MedianOf(deviations)
	excess := summary.leaderMagnitude - peerMedian
	denominator := excess + peerDispersion

	if denominator <= 0 {
		return summary
	}

	summary.leaderEvidence = excess / denominator

	if len(peerMagnitudes) > 0 {
		summary.divergence = summary.leaderEvidence *
			float64(nonconfirming) / float64(len(peerMagnitudes))
	}

	return summary
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
