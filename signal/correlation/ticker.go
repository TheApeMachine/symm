package correlation

import (
	"context"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Ticker is the asynchronous price-path correlation instrument. It holds no
state and no logic of its own: its entire behavior is one nomagique pipeline,
and Step is only the input and output boundary for the measurement the
workload's data management hands it.
*/
type Ticker struct {
	*runtime.System
	pipeline core.Primitive
	ID       int
}

func NewTicker(ctx context.Context) *Ticker {
	return &Ticker{
		System: runtime.NewSystem(ctx, "correlation:ticker"),
		pipeline: nomagique.NewNumber(
			nmcorrelation.NewGate(),
			nmcorrelation.NewFocal(),
			nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
			nmcorrelation.NewFold(),
			nmcorrelation.NewHistory(),
			nmcorrelation.NewRelative(),
			nmcorrelation.NewCorrelationVelocity(),
			nmcorrelation.NewEnergyVelocity(),
		),
	}
}

/*
Step supplies the arriving measurement to the pipeline and shapes the
pipeline's reduced facts back into the same measurement.
*/
func (ticker *Ticker) Step(m *data.Measurement[float64]) *data.Measurement[float64] {
	reading := nmcorrelation.Reading{Measurement: m}

	for range ticker.pipeline.Next(transport.NewOne(unsafe.Pointer(&reading)).Next(nil)) {
	}

	if err := ticker.pipeline.Error(); err != nil && m.Err == nil {
		m.Err = err

		return m
	}

	switch reading.State {
	case nmcorrelation.StateNeedsPrice, nmcorrelation.StateInvalid:
		return m
	case nmcorrelation.StateUnobserved:
		m.Provenance = map[string]string{"last_trade_price_state": "unobserved"}
		m.Finalize()

		return m
	case nmcorrelation.StateRegressed:
		m.Provenance = map[string]string{"event_time_state": "regressed"}
		m.Finalize()

		return m
	case nmcorrelation.StateTraded:
	}

	if !reading.Focal.Accepted {
		m.Provenance = map[string]string{"event_time_state": "regressed"}
		m.Finalize()

		return m
	}

	m.Metrics["last_price"] = m.Metrics["last_price"].Write(reading.Price.Value)
	m.Metrics["observation_count"] = m.Metrics["observation_count"].Write(reading.Focal.Count)

	pair, cohort, history := reading.Selected, reading.Cohort, reading.History

	if len(reading.Admitted) > 0 {
		m.Provenance = map[string]string{
			"peer":                       reading.SelectedSymbol,
			"pair_diagnostics_selection": "last_defined_peer_lexicographic",
		}

		m.Metrics["signed_correlation"] = m.Metrics["signed_correlation"].Write(pair.Dependence.Correlation)
		m.Metrics["absolute_correlation"] = m.Metrics["absolute_correlation"].Write(math.Abs(pair.Dependence.Correlation))
		m.Metrics["cohort_signed_correlation"] = m.Metrics["cohort_signed_correlation"].Write(cohort.SignedCorrelation)
		m.Metrics["cohort_absolute_correlation"] = m.Metrics["cohort_absolute_correlation"].Write(cohort.AbsoluteCorrelation)
		m.Metrics["covariance"] = m.Metrics["covariance"].Write(pair.Dependence.Covariance)
		m.Metrics["return_energy:reference"] = m.Metrics["return_energy:reference"].Write(pair.Dependence.RightEnergy)
		m.Metrics["return_energy:measured"] = m.Metrics["return_energy:measured"].Write(pair.Dependence.LeftEnergy)
		m.Metrics["return_energy_rate:reference"] = m.Metrics["return_energy_rate:reference"].Write(pair.Dependence.RightEnergyRate)
		m.Metrics["return_energy_rate:measured"] = m.Metrics["return_energy_rate:measured"].Write(pair.Dependence.LeftEnergyRate)
		m.Metrics["focal_return_energy_rate"] = m.Metrics["focal_return_energy_rate"].Write(pair.Dependence.LeftEnergyRate)
		m.Metrics["overlap_density"] = m.Metrics["overlap_density"].Write(pair.Dependence.OverlapDensity)
		m.Metrics["peer_return_energy_rate"] = m.Metrics["peer_return_energy_rate"].Write(cohort.PeerEnergyRate)
		m.Metrics["supported_return_count:measured"] = m.Metrics["supported_return_count:measured"].Write(pair.Dependence.LeftReturns)
		m.Metrics["supported_return_count:reference"] = m.Metrics["supported_return_count:reference"].Write(pair.Dependence.RightReturns)
		m.Metrics["overlap_pair_count"] = m.Metrics["overlap_pair_count"].Write(pair.Dependence.Support)
		m.Metrics["effective_sample_count"] = m.Metrics["effective_sample_count"].Write(pair.Dependence.Support)
		m.Metrics["shared_time"] = m.Metrics["shared_time"].Write(pair.Dependence.SharedTime)
		m.Metrics["cohort_peer_count"] = m.Metrics["cohort_peer_count"].Write(cohort.Peers)
		m.Metrics["cohort_effective_peer_count"] = m.Metrics["cohort_effective_peer_count"].Write(cohort.EffectivePeers)
		m.Metrics["relative_return_energy"] = m.Metrics["relative_return_energy"].Write(reading.Relative)
		m.Metrics["relative_cohort_return_energy"] = m.Metrics["relative_cohort_return_energy"].Write(reading.Relative)
		m.Metrics["correlation_baseline"] = m.Metrics["correlation_baseline"].Write(history.Baseline)
		m.Metrics["correlation_divergence"] = m.Metrics["correlation_divergence"].Write(history.Divergence)
		m.Metrics["correlation_zscore"] = m.Metrics["correlation_zscore"].Write(history.ZScore)
		m.Metrics["relative_return_energy_baseline"] = m.Metrics["relative_return_energy_baseline"].Write(reading.RelativeHistory.Baseline)
		m.Metrics["relative_return_energy_divergence"] = m.Metrics["relative_return_energy_divergence"].Write(reading.RelativeHistory.Residual)
		m.Metrics["relative_return_energy_zscore"] = m.Metrics["relative_return_energy_zscore"].Write(reading.RelativeHistory.ZScore)

		if pair.Fisher.Defined {
			m.Metrics["correlation_p_value"] = data.Metric[float64]{
				Label:     "correlation_p_value",
				Raw:       pair.Fisher.PValue,
				Unit:      data.UnitDimensionless,
				Timescale: data.TimescaleInstantaneous,
			}

			m.Metrics["correlation_standard_error_fisher"] = data.Metric[float64]{
				Label:     "correlation_standard_error_fisher",
				Raw:       pair.Fisher.StandardError,
				Unit:      data.UnitDimensionless,
				Timescale: data.TimescaleInstantaneous,
			}
		}

		if cohort.FisherDefined {
			m.Metrics["cohort_correlation_dispersion"] = data.Metric[float64]{
				Label:     "cohort_correlation_dispersion",
				Raw:       cohort.Dispersion,
				Unit:      data.UnitNat,
				Timescale: data.TimescaleInstantaneous,
			}
		}

		if reading.CorrelationVelocity.Defined {
			m.Metrics["correlation_velocity"] = data.Metric[float64]{
				Label:     "correlation_velocity",
				Raw:       reading.CorrelationVelocity.Rate,
				Unit:      data.UnitPerSecond,
				Timescale: data.TimescaleInstantaneous,
			}
		}

		if reading.EnergyVelocity.Defined {
			m.Metrics["relative_return_energy_velocity"] = data.Metric[float64]{
				Label:     "relative_return_energy_velocity",
				Raw:       reading.EnergyVelocity.Rate,
				Unit:      data.UnitPerSecond,
				Timescale: data.TimescaleInstantaneous,
			}
		}

		if history.Defined {
			m.Metadata[data.MetadataDivergence] = history.Divergence
			m.Metadata[data.MetadataSupport] = history.Count

			if history.VarianceDefined {
				m.Metadata[data.MetadataNoiseVariance] = history.Variance
			}
		}
	}

	m.Finalize()

	return m
}

func (ticker *Ticker) Identify() int      { return ticker.ID }
func (ticker *Ticker) SetIdentity(ID int) { ticker.ID = ID }

/*
Register returns the pre-allocated measurement every correlation tick flows
through: every metric the instrument can produce is declared, none valued.
*/
func (ticker *Ticker) Register() *data.Measurement[float64] {
	return data.NewMeasurement("correlation", map[string]data.Metric[float64]{
		"last_price": data.NewMetric[float64](
			"last_price", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"observation_count": data.NewMetric[float64](
			"observation_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"signed_correlation": data.NewMetric[float64](
			"signed_correlation", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"absolute_correlation": data.NewMetric[float64](
			"absolute_correlation", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"cohort_signed_correlation": data.NewMetric[float64](
			"cohort_signed_correlation", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"cohort_absolute_correlation": data.NewMetric[float64](
			"cohort_absolute_correlation", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"covariance": data.NewMetric[float64](
			"covariance", data.UnitNat, data.TimescaleInstantaneous, 0, 1,
		),
		"return_energy:reference": data.NewMetric[float64](
			"return_energy:reference", data.UnitNat, data.TimescaleInstantaneous, 0, 1,
		),
		"return_energy:measured": data.NewMetric[float64](
			"return_energy:measured", data.UnitNat, data.TimescaleInstantaneous, 0, 1,
		),
		"return_energy_rate:reference": data.NewMetric[float64](
			"return_energy_rate:reference", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"return_energy_rate:measured": data.NewMetric[float64](
			"return_energy_rate:measured", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"focal_return_energy_rate": data.NewMetric[float64](
			"focal_return_energy_rate", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"overlap_density": data.NewMetric[float64](
			"overlap_density", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"peer_return_energy_rate": data.NewMetric[float64](
			"peer_return_energy_rate", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"supported_return_count:measured": data.NewMetric[float64](
			"supported_return_count:measured", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"supported_return_count:reference": data.NewMetric[float64](
			"supported_return_count:reference", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"overlap_pair_count": data.NewMetric[float64](
			"overlap_pair_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"effective_sample_count": data.NewMetric[float64](
			"effective_sample_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"shared_time": data.NewMetric[float64](
			"shared_time", data.UnitSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"correlation_p_value": data.NewMetric[float64](
			"correlation_p_value", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"correlation_standard_error_fisher": data.NewMetric[float64](
			"correlation_standard_error_fisher", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"cohort_peer_count": data.NewMetric[float64](
			"cohort_peer_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"cohort_effective_peer_count": data.NewMetric[float64](
			"cohort_effective_peer_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"cohort_correlation_dispersion": data.NewMetric[float64](
			"cohort_correlation_dispersion", data.UnitNat, data.TimescaleInstantaneous, 0, 1,
		),
		"relative_return_energy": data.NewMetric[float64](
			"relative_return_energy", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"relative_cohort_return_energy": data.NewMetric[float64](
			"relative_cohort_return_energy", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"correlation_baseline": data.NewMetric[float64](
			"correlation_baseline", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"correlation_divergence": data.NewMetric[float64](
			"correlation_divergence", data.UnitNat, data.TimescaleInstantaneous, 0, 1,
		),
		"correlation_zscore": data.NewMetric[float64](
			"correlation_zscore", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"correlation_velocity": data.NewMetric[float64](
			"correlation_velocity", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"relative_return_energy_baseline": data.NewMetric[float64](
			"relative_return_energy_baseline", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"relative_return_energy_divergence": data.NewMetric[float64](
			"relative_return_energy_divergence", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"relative_return_energy_zscore": data.NewMetric[float64](
			"relative_return_energy_zscore", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"relative_return_energy_velocity": data.NewMetric[float64](
			"relative_return_energy_velocity", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
	})
}
