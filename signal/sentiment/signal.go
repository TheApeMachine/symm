package sentiment

import (
	"context"
	"math"
	"time"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
Signal measures global market conviction from breadth and leadership
performance. Categories belong in logic; this signal emits numerical scores only.
*/
type Signal struct {
	tickerIn chan []kraken.TickerData
	bookIn   chan []kraken.BookData
	tradeIn  chan []kraken.TradeData
	ctx      context.Context
	cancel   context.CancelFunc
	ui       chan []byte
}

/*
NewSignal creates sentiment measurement state for central market cuts so every
tick can compare breadth with current leadership.
*/
func NewSignal(ctx context.Context, ui chan []byte) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		tickerIn: make(chan []kraken.TickerData, 64),
		bookIn:   make(chan []kraken.BookData, 64),
		tradeIn:  make(chan []kraken.TradeData, 64),
		ctx:      ctx,
		cancel:   cancel,
		ui:       ui,
	}

	return signal
}

/*
Publish sends one compact datura frame per symbol with nested metrics so the UI
receives one observation map rather than one envelope per metric.
*/
func (signal *Signal) Publish(measurements []*types.Measurement) {
	select {
	case signal.ui <- datura.Map[any]{
		"measurements": types.ForPublish(measurements),
	}.Marshal():
	default:
	}
}

/*
Calculate converts the receiver's current market input into typed measurements
so downstream logic consumes explicit evidence.
*/
func (signal *Signal) Calculate(
	tickers []kraken.TickerData,
	trades []kraken.TradeData,
	books []kraken.BookData,
) ([]*types.Measurement, error) {
	crossSection := types.NewCrossSection()
	if len(tickers) > 0 {
		crossSection.Measure(tickers)
	}

	out := make([]*types.Measurement, 0, len(tickers)*9)

	if crossSection == nil {
		return out, nil
	}

	leader, leadershipThreshold := crossSection.Leadership()
	breadth := crossSection.Breadth()
	cohortSize := 0
	leaderChange := 0.0
	totalChange := 0.0
	spreadUncertainty := 0.0
	positiveDisplacements := 0
	negativeDisplacements := 0
	minimumDisplacement := math.Inf(1)

	crossSection.Metrics.Range(func(_, value any) bool {
		metric := value.(types.SymbolMetric)
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

	for _, row := range tickers {
		change := row.ChangePct / 100
		leaderStrength := 0.0
		leaderEvidence := 0.0
		relativeLead := 0.0
		divergentScore := 0.0
		isLeader := leader == row.Symbol && leaderMagnitude > 0

		if isLeader {
			leaderStrength = leaderMagnitude
			leaderEvidence = (leaderMagnitude - leadershipThreshold) / leaderMagnitude
			relativeLead = leaderMagnitude / totalChange
			divergentScore = divergenceScore
		}

		strength := math.Max(surgeScore, math.Max(divergentScore, slumpScore))
		specs := []struct {
			metric types.MetricType
			raw    float64
		}{
			{types.MetricChange, change},
			{types.MetricBreadth, breadth},
			{types.MetricLeaderStrength, leaderStrength},
			{types.MetricLeaderEvidence, leaderEvidence},
			{types.MetricRelativeLead, relativeLead},
			{types.MetricSurgeScore, surgeScore},
			{types.MetricDivergentScore, divergentScore},
			{types.MetricSlumpScore, slumpScore},
			{types.MetricStrength, strength},
		}

		for _, spec := range specs {
			out = append(out, &types.Measurement{
				Source:   types.SourceSentiment,
				Metric:   spec.metric,
				Stream:   types.Sentiment,
				Symbol:   row.Symbol,
				At:       row.Timestamp,
				Unit:     types.UnitDimensionless,
				Raw:      spec.raw,
				Validity: validity,
			})
		}
	}

	return out, nil
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()

	return nil
}

/*
Tickers returns the ticker ingress channel.
*/
func (signal *Signal) Tickers() chan []kraken.TickerData {
	return signal.tickerIn
}

/*
Books returns the book ingress channel.
*/
func (signal *Signal) Books() chan []kraken.BookData {
	return signal.bookIn
}

/*
Trades returns the trade ingress channel.
*/
func (signal *Signal) Trades() chan []kraken.TradeData {
	return signal.tradeIn
}

/*
Measure consumes ingress channels and sends measurements on out.
*/
func (signal *Signal) Measure() chan []*types.Measurement {
	out := make(chan []*types.Measurement, 64)

	go func() {
		defer close(out)

		for {
			select {
			case <-signal.ctx.Done():
				return
			case rows := <-signal.tickerIn:
				measured, err := signal.Calculate(rows, nil, nil)

				if err != nil {
					errnie.Error(err)
					continue
				}

				if len(measured) == 0 {
					continue
				}

				select {
				case out <- measured:
					signal.Publish(measured)
				default:
				}
			case rows := <-signal.bookIn:
				measured, err := signal.Calculate(nil, nil, rows)

				if err != nil {
					errnie.Error(err)
					continue
				}

				if len(measured) == 0 {
					continue
				}

				select {
				case out <- measured:
					signal.Publish(measured)
				default:
				}
			case rows := <-signal.tradeIn:
				measured, err := signal.Calculate(nil, rows, nil)

				if err != nil {
					errnie.Error(err)
					continue
				}

				if len(measured) == 0 {
					continue
				}

				select {
				case out <- measured:
					signal.Publish(measured)
				default:
				}
			default:
				time.Sleep(10 * time.Millisecond)
			}
		}
	}()

	return out
}
