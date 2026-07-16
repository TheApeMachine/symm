package sentiment

import (
	"context"
	"math"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Sentiment measures global market conviction from breadth and leadership
performance. Categories belong in logic; this signal emits numerical scores only.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	ticker *Ticker
	ui     chan []byte
}

/*
NewSignal creates sentiment measurement state and subscribes its ticker input
so every tick can compare breadth with current leadership.
*/
func NewSignal(ctx context.Context, api *websocket.API, ui chan []byte) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		ticker: NewTicker(ctx, api),
		ui:     ui,
	}
}

/*
Publish sends one small datura frame to the UI the moment this signal has
measured its evidence, mirroring broker.Balance.Publish.
*/
func (signal *Signal) Publish(measurements []*types.Measurement) {
	filtered := types.FilterLatest(measurements)

	if len(filtered) == 0 {
		return
	}

	select {
	case signal.ui <- datura.Map[any]{
		"measurements": filtered,
	}.Marshal():
	default:
	}
}

/*
Capture freezes the ticker journal before planner starts measuring signals so
breadth and leadership use one stable market surface.
*/
func (signal *Signal) Capture(at time.Time) error {
	return signal.ticker.cache.Capture(at)
}

/*
Measure converts the receiver's current market input into typed measurements
so downstream logic consumes explicit evidence.
*/
func (signal *Signal) Measure(
	thesis *types.Thesis,
) *types.Thesis {
	tickers, err := signal.ticker.cache.Frame(thesis.At)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"sentiment: ticker frame failed",
			err,
		))
		return thesis
	}

	out := make([]*types.Measurement, 0, len(tickers)*9)

	if thesis.CrossSection == nil {
		return thesis
	}

	thesis.CrossSection.Measure(tickers)
	leader, leadershipThreshold := thesis.CrossSection.Leadership()
	breadth := thesis.CrossSection.Breadth()

	for _, row := range tickers {
		change := row.ChangePct / 100
		leaderStrength := 0.0
		leaderEvidence := 0.0
		relativeLead := 0.0

		isLeader := leader == row.Symbol && leadershipThreshold > 0 &&
			math.Abs(change) >= leadershipThreshold

		if isLeader {
			leaderStrength = math.Abs(change)
			leaderEvidence = leaderStrength / leadershipThreshold
			relativeLead = 1
		}

		leaderMass := leaderEvidence / (1 + leaderEvidence)
		surgeScore := breadth * leaderMass * math.Max(relativeLead, 1-leaderMass)
		divergentScore := (1 - breadth) * relativeLead * leaderMass
		slumpScore := (1 - breadth) * (1 - relativeLead) / (1 + leaderMass)
		strength := math.Max(surgeScore, math.Max(divergentScore, slumpScore))

		validity := types.MeasurementValidity{
			State:     types.ValidityValid,
			Readiness: types.ReadinessObservation,
		}

		out = append(out,
			&types.Measurement{
				Source:   types.SourceSentiment,
				Metric:   types.MetricChange,
				Stream:   types.Sentiment,
				Symbol:   row.Symbol,
				At:       row.Timestamp,
				Unit:     types.UnitDimensionless,
				Raw:      change,
				Validity: validity,
			},
			&types.Measurement{
				Source:   types.SourceSentiment,
				Metric:   types.MetricBreadth,
				Stream:   types.Sentiment,
				Symbol:   row.Symbol,
				At:       row.Timestamp,
				Unit:     types.UnitDimensionless,
				Raw:      breadth,
				Validity: validity,
			},
			&types.Measurement{
				Source:   types.SourceSentiment,
				Metric:   types.MetricLeaderStrength,
				Stream:   types.Sentiment,
				Symbol:   row.Symbol,
				At:       row.Timestamp,
				Unit:     types.UnitDimensionless,
				Raw:      leaderStrength,
				Validity: validity,
			},
			&types.Measurement{
				Source:   types.SourceSentiment,
				Metric:   types.MetricLeaderEvidence,
				Stream:   types.Sentiment,
				Symbol:   row.Symbol,
				At:       row.Timestamp,
				Unit:     types.UnitDimensionless,
				Raw:      leaderEvidence,
				Validity: validity,
			},
			&types.Measurement{
				Source:   types.SourceSentiment,
				Metric:   types.MetricRelativeLead,
				Stream:   types.Sentiment,
				Symbol:   row.Symbol,
				At:       row.Timestamp,
				Unit:     types.UnitDimensionless,
				Raw:      relativeLead,
				Validity: validity,
			},
			&types.Measurement{
				Source:   types.SourceSentiment,
				Metric:   types.MetricSurgeScore,
				Stream:   types.Sentiment,
				Symbol:   row.Symbol,
				At:       row.Timestamp,
				Unit:     types.UnitDimensionless,
				Raw:      surgeScore,
				Validity: validity,
			},
			&types.Measurement{
				Source:   types.SourceSentiment,
				Metric:   types.MetricDivergentScore,
				Stream:   types.Sentiment,
				Symbol:   row.Symbol,
				At:       row.Timestamp,
				Unit:     types.UnitDimensionless,
				Raw:      divergentScore,
				Validity: validity,
			},
			&types.Measurement{
				Source:   types.SourceSentiment,
				Metric:   types.MetricSlumpScore,
				Stream:   types.Sentiment,
				Symbol:   row.Symbol,
				At:       row.Timestamp,
				Unit:     types.UnitDimensionless,
				Raw:      slumpScore,
				Validity: validity,
			},
			&types.Measurement{
				Source:   types.SourceSentiment,
				Metric:   types.MetricStrength,
				Stream:   types.Sentiment,
				Symbol:   row.Symbol,
				At:       row.Timestamp,
				Unit:     types.UnitDimensionless,
				Raw:      strength,
				Validity: validity,
			},
		)
	}

	thesis.Measurements = append(thesis.Measurements, out...)
	signal.Publish(out)

	return thesis
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() (err error) {
	err = errnie.Error(errnie.Err(
		errnie.Internal,
		"signal: close failed",
		nil,
	))

	signal.cancel()
	return err
}
