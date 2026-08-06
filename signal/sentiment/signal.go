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
					measurements := signal.Measure(thesis)

					if len(measurements) > 0 {
						thesis.AppendMeasurements(measurements)
						utils.Fanout(signal.subscribers, signal.Name(), thesis)
					}
				}
			}
		}
	}()
}

/*
Measure produces the Measurements for the sentiment signal.
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

	if !statistics.scaleReady {
		validity.State = types.ValidityProvisional
		validity.Reason = appendReason(
			validity.Reason,
			"peer return magnitude scale unavailable",
		)
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
		metrics, normalized := sentimentMetrics(
			map[types.MetricType]float64{
				types.MetricChange:         change,
				types.MetricBreadth:        statistics.breadth,
				types.MetricLeaderStrength: leaderStrength,
				types.MetricLeaderEvidence: leaderEvidence,
				types.MetricRelativeLead:   relativeLead,
				types.MetricSurgeScore:     statistics.surge,
				types.MetricDivergentScore: peerDivergenceScore,
				types.MetricSlumpScore:     statistics.slump,
				types.MetricStrength:       strength,
			},
			statistics.magnitudeBaseline,
			validity.State == types.ValidityValid,
		)
		measurementValidity := validity

		if validity.State == types.ValidityValid && !normalized {
			measurementValidity.State = types.ValidityInvalid
			measurementValidity.Reason = "sentiment normalization contract violated"
		}

		measurement := &types.Measurement{
			Source:   types.SourceSentiment,
			Symbol:   peer.symbol,
			At:       peer.observation.at,
			Validity: measurementValidity,
			Scale:    scale,
			Metrics:  metrics,
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
	leader            string
	leaderMagnitude   float64
	leaderEvidence    float64
	relativeLead      float64
	breadth           float64
	surge             float64
	slump             float64
	divergence        float64
	magnitudeBaseline float64
	scaleReady        bool
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
		summary.magnitudeBaseline = medianMagnitude
		summary.scaleReady = true
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

/*
sentimentMetrics normalizes raw log returns and leader magnitude by the current
cohort's median absolute return. Breadth and the remaining evidence scores are
already signed or unsigned cohort fractions derived from that same cut.
*/
func sentimentMetrics(
	readings map[types.MetricType]float64,
	magnitudeBaseline float64,
	ready bool,
) (map[string]types.MetricSample, bool) {
	metrics := make(map[string]types.MetricSample, len(readings))
	valid := ready

	for metric, raw := range readings {
		sample := types.MetricSample{Raw: raw, Unit: types.UnitDimensionless}

		if ready {
			sample.Normalized = normalizedSentimentMetric(
				metric,
				raw,
				magnitudeBaseline,
			)

			if sample.Normalized == nil {
				valid = false
			}
		}

		metrics[types.MetricKey(metric, types.SideNone)] = sample
	}

	return metrics, valid
}

func normalizedSentimentMetric(
	metric types.MetricType,
	raw float64,
	magnitudeBaseline float64,
) *float64 {
	if math.IsNaN(raw) || math.IsInf(raw, 0) {
		return nil
	}

	if metric == types.MetricChange || metric == types.MetricLeaderStrength {
		if magnitudeBaseline <= 0 || math.IsNaN(magnitudeBaseline) ||
			math.IsInf(magnitudeBaseline, 0) {
			return nil
		}

		value := raw / magnitudeBaseline

		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil
		}

		return &value
	}

	if metric == types.MetricBreadth {
		if raw < -1 || raw > 1 {
			return nil
		}

		value := raw

		return &value
	}

	if raw < 0 || raw > 1 {
		return nil
	}

	value := raw

	return &value
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
