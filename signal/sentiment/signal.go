package sentiment

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/kraken/websocket"
	signalshared "github.com/theapemachine/symm/signal"
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
	if signal.subscribers == nil {
		signal.subscribers = &sync.Map{}
	}

	return signalshared.Subscribe(
		&signal.subscribeMu,
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
						types.Stamp{At: time.Now(), Entity: types.MarketTicker},
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
	measurements := make([]*types.Measurement, 0, 64)
	out := make([]*types.Measurement, 0)

	if thesis.CrossSection == nil {
		return measurements
	}

	if len(tickers) > 0 {
		thesis.CrossSection.Measure(tickers)
	}

	leader, leadershipThreshold := thesis.CrossSection.Leadership()
	breadth := thesis.CrossSection.Breadth()
	cohortSize := 0
	leaderChange := 0.0
	totalChange := 0.0
	spreadUncertainty := 0.0
	positiveDisplacements := 0
	negativeDisplacements := 0
	minimumDisplacement := math.Inf(1)
	peers := make([]types.SymbolMetric, 0, 64)

	thesis.CrossSection.Metrics.Range(func(_, value any) bool {
		metric := value.(types.SymbolMetric)
		peers = append(peers, metric)
		absoluteChange := math.Abs(metric.LatestChange)
		displacement := absoluteChange - metric.RelativeSpread
		cohortSize++
		totalChange += absoluteChange
		spreadUncertainty += metric.RelativeSpread

		if metric.Symbol == leader {
			leaderChange = metric.LatestChange
		}

		if displacement <= 0 {
			return true
		}

		minimumDisplacement = math.Min(minimumDisplacement, displacement)

		if metric.LatestChange > 0 {
			positiveDisplacements++
		}

		if metric.LatestChange < 0 {
			negativeDisplacements++
		}

		return true
	})

	surgeScore := 0.0
	slumpScore := 0.0

	if cohortSize > 0 && positiveDisplacements == cohortSize && leadershipThreshold > 0 {
		surgeScore = minimumDisplacement / leadershipThreshold
	}

	if cohortSize > 0 && negativeDisplacements == cohortSize && leadershipThreshold > 0 {
		slumpScore = minimumDisplacement / leadershipThreshold
	}

	leaderMagnitude := math.Abs(leaderChange)
	divergenceScore := 0.0

	if leaderChange > 0 {
		peerChange := totalChange - leaderMagnitude
		dominance := leaderMagnitude - peerChange - spreadUncertainty

		if dominance > 0 {
			divergenceScore = dominance / leaderMagnitude
		}
	}

	if divergenceScore > 0 {
		surgeScore = 0
	}

	validity := types.MeasurementValidity{
		State:     types.ValidityValid,
		Readiness: types.ReadinessObservation,
	}

	if cohortSize < 2 {
		validity.State = types.ValidityProvisional
		validity.Reason = "peer return cohort unavailable"
	}

	for _, peer := range peers {
		change := peer.LatestChange
		leaderStrength := 0.0
		leaderEvidence := 0.0
		relativeLead := 0.0
		peerDivergenceScore := 0.0
		isLeader := leader == peer.Symbol && leaderMagnitude > 0

		if isLeader {
			leaderStrength = leaderMagnitude

			if leaderMagnitude > 0 {
				leaderEvidence = (leaderMagnitude - leadershipThreshold) / leaderMagnitude
			}

			if totalChange > 0 {
				relativeLead = leaderMagnitude / totalChange
			}

			peerDivergenceScore = divergenceScore
		}

		strength := math.Max(surgeScore, math.Max(peerDivergenceScore, slumpScore))

		measurement := &types.Measurement{
			Source:   types.SourceSentiment,
			Symbol:   peer.Symbol,
			At:       peer.At,
			Maturity: float64(thesis.Tick),
			Validity: validity,
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricChange, types.SideNone): {
					Raw:        change,
					Normalized: types.NormalizeFinite(change),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricBreadth, types.SideNone): {
					Raw:        breadth,
					Normalized: types.NormalizeFinite(breadth),
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
					Raw:        surgeScore,
					Normalized: types.NormalizeFinite(surgeScore),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricDivergentScore, types.SideNone): {
					Raw:        peerDivergenceScore,
					Normalized: types.NormalizeFinite(peerDivergenceScore),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricSlumpScore, types.SideNone): {
					Raw:        slumpScore,
					Normalized: types.NormalizeFinite(slumpScore),
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

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
