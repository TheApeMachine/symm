package leadlag

import (
	"context"
	"math"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
LeadLag is the Anchor perspective, measuring temporal correlation between the
cross-section leader and each follower. Categories belong in logic; this signal
emits numerical scores only.
*/
type Signal struct {
	ctx     context.Context
	cancel  context.CancelFunc
	section *Section
	ui      chan []byte
}

func (signal *Signal) Measure(thesis *types.Thesis) chan []*types.Measurement {
	out := make(chan []*types.Measurement)

	go func() {
		defer close(out)

		measurements, err := signal.Calculate(thesis.Market())

		if err != nil {
			errnie.Error(err)
			out <- nil
			return
		}

		out <- measurements
		signal.Publish(measurements)
	}()

	return out
}

/*
NewSignal creates lead-lag measurement state for central market cuts so
temporal relationships persist across Thesis ticks.
*/
func NewSignal(ctx context.Context, api *websocket.API, ui chan []byte) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:     ctx,
		cancel:  cancel,
		section: NewSection(),
		ui:      ui,
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
Calculate converts the receiver's current market input into typed measurements
so downstream logic consumes explicit evidence.
*/
func (signal *Signal) Calculate(
	frame *types.MarketFrame,
) ([]*types.Measurement, error) {
	rows := frame.Tickers
	out := make([]*types.Measurement, 0, len(rows))
	anchor, _ := frame.CrossSection.Leadership()

	if anchor != "" {
		signal.section.SetAnchor(anchor)

		for _, row := range rows {
			if row.Timestamp.IsZero() {
				continue
			}

			if row.Last == nil {
				continue
			}

			lastPrice := row.Last.Float64()

			if lastPrice <= 0 {
				continue
			}

			signal.section.ObservePrice(row.Symbol, lastPrice, row.Timestamp)
		}

		for _, row := range rows {
			features := signal.section.Features(row.Symbol)

			if features.Price <= 0 {
				continue
			}

			lagFraction := 0.0
			lagCorrelation := 0.0
			contempCorrelation := 0.0
			signedLagCorrelation := 0.0
			signedContempCorrelation := 0.0

			if features.LagOK && features.SampleCount > 0 {
				dynamicMax := signal.section.maxLagBars(features.SampleCount)

				if dynamicMax > 0 {
					lagFraction = math.Abs(float64(features.LagBars)) / float64(dynamicMax)
				}

				signedLagCorrelation = features.LagCorr
				lagCorrelation = math.Abs(features.LagCorr)
			}

			if features.ContempOK {
				signedContempCorrelation = features.ContempCorr
				contempCorrelation = math.Abs(features.ContempCorr)
			}

			correlation := min(math.Max(contempCorrelation, lagCorrelation), 1)

			lagDominates := max(0, min(1, (lagCorrelation-contempCorrelation)*1e9))
			signedCorrelation := min(max(
				signedContempCorrelation+lagDominates*(signedLagCorrelation-signedContempCorrelation),
				-1,
			), 1)

			sampleSupport := 0.0

			if features.SampleCount > 0 {
				shortWindow, _, err := statistic.ResolveWindows(
					make([]float64, features.SampleCount),
					0,
					0,
				)

				if err == nil && shortWindow > 0 {
					sampleSupport = float64(features.SampleCount) / float64(shortWindow)
				}
			}

			anchorActive := 0.1

			if features.MoveMoved ||
				(features.StallMargin > 0 && lagFraction > 0) ||
				features.ContempOK ||
				features.LagOK {
				anchorActive = 1
			}

			stallDamp := 1.0

			if features.MoveMoved {
				stallDamp = 0
			}

			stallMargin := math.Min(1, math.Max(0, features.StallMargin))
			noLag := 1 - lagFraction
			uncorrelated := 1 - correlation
			lagEvidence := lagCorrelation * lagFraction
			syncEvidence := contempCorrelation * noLag
			decoupledEvidence := uncorrelated * (1 - stallMargin)
			stallEvidence := stallMargin * uncorrelated * noLag * stallDamp

			inefficient := sampleSupport * anchorActive * lagEvidence * (1 - stallMargin)
			syncScore := sampleSupport * anchorActive * syncEvidence * (1 - stallMargin)
			decoupled := sampleSupport * anchorActive * decoupledEvidence
			stall := sampleSupport * anchorActive * stallEvidence
			strength := max(max(inefficient, syncScore), max(decoupled, stall))

			validity := types.MeasurementValidity{
				State:     types.ValidityValid,
				Readiness: types.ReadinessObservation,
			}

			out = append(out,
				&types.Measurement{
					Source:   types.SourceLeadLag,
					Metric:   types.MetricCorrelation,
					Stream:   types.LeadLag,
					Symbol:   row.Symbol,
					At:       row.Timestamp,
					Unit:     types.UnitDimensionless,
					Raw:      correlation,
					Validity: validity,
				},
				&types.Measurement{
					Source:   types.SourceLeadLag,
					Metric:   types.MetricSignedCorrelation,
					Stream:   types.LeadLag,
					Symbol:   row.Symbol,
					At:       row.Timestamp,
					Unit:     types.UnitDimensionless,
					Raw:      signedCorrelation,
					Validity: validity,
				},
				&types.Measurement{
					Source:   types.SourceLeadLag,
					Metric:   types.MetricSignedContempCorrelation,
					Stream:   types.LeadLag,
					Symbol:   row.Symbol,
					At:       row.Timestamp,
					Unit:     types.UnitDimensionless,
					Raw:      signedContempCorrelation,
					Validity: validity,
				},
				&types.Measurement{
					Source:   types.SourceLeadLag,
					Metric:   types.MetricSignedLagCorrelation,
					Stream:   types.LeadLag,
					Symbol:   row.Symbol,
					At:       row.Timestamp,
					Unit:     types.UnitDimensionless,
					Raw:      signedLagCorrelation,
					Validity: validity,
				},
				&types.Measurement{
					Source:   types.SourceLeadLag,
					Metric:   types.MetricLagFraction,
					Stream:   types.LeadLag,
					Symbol:   row.Symbol,
					At:       row.Timestamp,
					Unit:     types.UnitDimensionless,
					Raw:      lagFraction,
					Validity: validity,
				},
				&types.Measurement{
					Source:   types.SourceLeadLag,
					Metric:   types.MetricSampleSupport,
					Stream:   types.LeadLag,
					Symbol:   row.Symbol,
					At:       row.Timestamp,
					Unit:     types.UnitDimensionless,
					Raw:      sampleSupport,
					Validity: validity,
				},
				&types.Measurement{
					Source:   types.SourceLeadLag,
					Metric:   types.MetricInefficient,
					Stream:   types.LeadLag,
					Symbol:   row.Symbol,
					At:       row.Timestamp,
					Unit:     types.UnitDimensionless,
					Raw:      inefficient,
					Validity: validity,
				},
				&types.Measurement{
					Source:   types.SourceLeadLag,
					Metric:   types.MetricSync,
					Stream:   types.LeadLag,
					Symbol:   row.Symbol,
					At:       row.Timestamp,
					Unit:     types.UnitDimensionless,
					Raw:      syncScore,
					Validity: validity,
				},
				&types.Measurement{
					Source:   types.SourceLeadLag,
					Metric:   types.MetricDecoupled,
					Stream:   types.LeadLag,
					Symbol:   row.Symbol,
					At:       row.Timestamp,
					Unit:     types.UnitDimensionless,
					Raw:      decoupled,
					Validity: validity,
				},
				&types.Measurement{
					Source:   types.SourceLeadLag,
					Metric:   types.MetricStall,
					Stream:   types.LeadLag,
					Symbol:   row.Symbol,
					At:       row.Timestamp,
					Unit:     types.UnitDimensionless,
					Raw:      stall,
					Validity: validity,
				},
				&types.Measurement{
					Source:   types.SourceLeadLag,
					Metric:   types.MetricStrength,
					Stream:   types.LeadLag,
					Symbol:   row.Symbol,
					At:       row.Timestamp,
					Unit:     types.UnitDimensionless,
					Raw:      strength,
					Validity: validity,
				},
			)
		}
	}

	return out, nil
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
