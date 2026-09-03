package correlation

import (
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
Ticker is the per-symbol price-path entity.

It connects market data to one nomagique pipeline per focal symbol and names
the readings that pipeline produces. It holds no mathematics and no estimator
state of its own.
*/
type Ticker struct {
	mutex sync.Mutex

	// paths is the retained observation store per symbol. Correlation is
	// cross-sectional, so a focal symbol is measured against every other path.
	paths map[string]*nmcorrelation.Path

	// pipelines holds one composed pipeline per focal symbol, so each symbol's
	// estimators accumulate their own history.
	pipelines map[string]*pipeline
}

/*
pipeline is one focal symbol's complete composition, declared once in
newPipeline and stepped once per peer.
*/
type pipeline struct {
	hayashi nmcorrelation.Hayashi
	fisher  nmcorrelation.Fisher
	cohort  nmcorrelation.Cohort

	// The signed correlation's causal history lives in Fisher coordinates,
	// where its dispersion is stationary; the baseline maps back into [-1, 1].
	signedCorrelation nmcorrelation.FisherEstimator
	relativeEnergy    equation.CausalResidual

	correlationVelocity temporal.Velocity
	energyVelocity      temporal.Velocity
	eventClock          temporal.Clock

	pairwise *nomagique.Pipeline
	cohortal *nomagique.Pipeline
}

/*
newPipeline declares one focal symbol's whole measurement as two
compositions: one stepped per peer, one stepped once the cohort is complete.

pairwise runs per peer:
  - A estimates the asynchronous correlation against that peer.
  - B tests its significance, reading support back out of the estimate.
  - C folds the peer into the cross-sectional accumulator, weighted by the
    overlap support it actually carried.

cohortal runs once the peers are folded, advancing the causal estimators over
the cohort's own readings. Every branch is a Tap, which returns 0, so the
carrier passes through each Split uncorrupted (Law of Sinks).
*/
func newPipeline(focal *nmcorrelation.Path, peer *nmcorrelation.Path) *pipeline {
	built := &pipeline{}

	pair := &built.hayashi
	pair.Left = focal
	pair.Right = peer

	built.fisher.Support = &nomagique.Tap{Read: pair.Support}

	built.cohort.Support = &nomagique.Tap{Read: pair.Support}
	built.cohort.PeerEnergy = &nomagique.Tap{Read: pair.RightEnergyRate}

	cohort := &built.cohort

	built.correlationVelocity.Source = &nomagique.Tap{Read: cohort.SignedCorrelation}
	built.correlationVelocity.Clock = &built.eventClock
	built.energyVelocity.Source = &nomagique.Tap{Read: built.relativeEnergyOf}
	built.energyVelocity.Clock = &built.eventClock

	built.pairwise = nomagique.Number(&nomagique.Chain{
		A: pair,
		B: &built.fisher,
		C: &nomagique.Tap{Read: pair.Correlation, Into: cohort},
	})

	built.cohortal = nomagique.Number(&nomagique.Split{
		A: &nomagique.Split{
			A: &nomagique.Tap{
				Read: cohort.SignedCorrelation,
				Into: &built.signedCorrelation,
			},
			B: &nomagique.Tap{
				Read: built.relativeEnergyOf,
				Into: &built.relativeEnergy,
			},
		},
		B: &nomagique.Split{
			A: &built.correlationVelocity,
			B: &built.energyVelocity,
		},
	})

	return built
}

/*
relativeEnergyOf reads the focal path's return energy as a multiple of its
cohort's. The focal energy is the Hayashi stage's left-hand reading and the
cohort supplies the peer average, so the ratio is read from the two nodes
rather than tracked by the signal.
*/
func (built *pipeline) relativeEnergyOf() nmtypes.Number {
	peerEnergy := built.cohort.PeerEnergyRate()

	if peerEnergy <= 0 {
		return 0
	}

	return built.hayashi.LeftEnergyRate() / peerEnergy
}

/*
NewTicker constructs the Ticker entity.
*/
func NewTicker() *Ticker {
	return &Ticker{
		paths:     make(map[string]*nmcorrelation.Path),
		pipelines: make(map[string]*pipeline),
	}
}

/*
Step receives one market data point, records it into the symbol's retained
path, steps that symbol's pipeline against every other observed path, and
names exactly one Measurement from the readings the pipeline exposes.

An explicit zero last means no recent trade price was observed: it produces
an undefined, zero-support measurement without entering the path, so an
untraded quote never becomes evidence.
*/
func (ticker *Ticker) Step(tickerData kraken.TickerData) *data.Measurement[float64] {
	if tickerData.Last == nil {
		return &data.Measurement[float64]{Err: fmt.Errorf(
			"correlation: ticker requires a last price",
		)}
	}

	last := tickerData.Last.Float64()

	if last < 0 {
		return &data.Measurement[float64]{Err: fmt.Errorf(
			"correlation: ticker last price must be non-negative",
		)}
	}

	measurement := data.NewMeasurement[float64](
		tickerData.Symbol+":correlation:"+tickerData.Timestamp.Format(time.RFC3339Nano),
		tickerData.Symbol,
		"correlation",
		tickerData.Timestamp,
		tickerData.Timestamp,
	)

	if last == 0 {
		measurement.Metadata = map[string]float64{data.MetadataSupport: 0}
		measurement.Provenance = map[string]string{
			"last_trade_price_state": "unobserved",
		}
		measurement.Finalize()

		return measurement
	}

	ticker.mutex.Lock()
	defer ticker.mutex.Unlock()

	focal := ticker.pathFor(tickerData.Symbol)

	if !focal.Observe(tickerData.Timestamp.UnixNano(), nmtypes.Number(last)) {
		measurement.Err = fmt.Errorf(
			"correlation: ticker event time regressed for %s", tickerData.Symbol,
		)

		return measurement
	}

	putMetric(measurement, "last_price", nmtypes.Number(last),
		data.UnitRate, data.TimescaleInstantaneous)
	putMetric(measurement, "observation_count", nmtypes.Number(focal.Len()),
		data.UnitCount, data.TimescaleInstantaneous)

	built := ticker.step(tickerData.Symbol, focal, tickerData.Timestamp)

	if built == nil || !built.cohort.Ready() {
		measurement.Metadata = map[string]float64{data.MetadataSupport: 0}
		measurement.Finalize()

		return measurement
	}

	built.project(measurement)

	if from, _, ok := focal.Span(); ok {
		measurement.From = time.Unix(0, from)
	}

	measurement.Finalize()

	return measurement
}

func (ticker *Ticker) pathFor(symbol string) *nmcorrelation.Path {
	path, found := ticker.paths[symbol]

	if !found {
		path = &nmcorrelation.Path{}
		ticker.paths[symbol] = path
	}

	return path
}

/*
step folds every peer into the focal symbol's cross-sectional accumulator,
then advances the cohort-level estimators once.
*/
func (ticker *Ticker) step(
	symbol string,
	focal *nmcorrelation.Path,
	at time.Time,
) *pipeline {
	built, found := ticker.pipelines[symbol]

	if !found {
		built = newPipeline(focal, nil)
		ticker.pipelines[symbol] = built
	}

	built.cohort.Reset()
	built.eventClock.Observe(at)

	folded := false

	for peerSymbol, peer := range ticker.paths {
		if peerSymbol == symbol {
			continue
		}

		// The focal path is measured against each peer in turn; only the peer
		// slot changes between steps.
		built.hayashi.Right = peer
		built.pairwise.Step(0)

		folded = true
	}

	if !folded {
		return nil
	}

	built.cohortal.Step(0)

	return built
}

/*
project names every reading the composition exposes. Each value is read from
a node; the signal derives none of them.
*/
func (built *pipeline) project(measurement *data.Measurement[float64]) {
	pair := &built.hayashi
	cohort := &built.cohort

	putMetric(measurement, "signed_correlation", cohort.SignedCorrelation(),
		data.UnitDimensionless, data.TimescaleInstantaneous)
	putMetric(measurement, "absolute_correlation", cohort.AbsoluteCorrelation(),
		data.UnitDimensionless, data.TimescaleInstantaneous)
	putMetric(measurement, "cohort_signed_correlation", cohort.SignedCorrelation(),
		data.UnitDimensionless, data.TimescaleInstantaneous)
	putMetric(measurement, "cohort_absolute_correlation", cohort.AbsoluteCorrelation(),
		data.UnitDimensionless, data.TimescaleInstantaneous)

	putMetric(measurement, "covariance", pair.Covariance(),
		data.UnitNat, data.TimescaleInstantaneous)
	putMetric(measurement, "return_energy:reference", pair.RightVariance(),
		data.UnitNat, data.TimescaleInstantaneous)
	putMetric(measurement, "return_energy:measured", pair.LeftVariance(),
		data.UnitNat, data.TimescaleInstantaneous)
	putMetric(measurement, "return_energy_rate:reference", pair.RightEnergyRate(),
		data.UnitPerSecond, data.TimescaleInstantaneous)
	putMetric(measurement, "return_energy_rate:measured", pair.LeftEnergyRate(),
		data.UnitPerSecond, data.TimescaleInstantaneous)
	putMetric(measurement, "peer_return_energy_rate", cohort.PeerEnergyRate(),
		data.UnitPerSecond, data.TimescaleInstantaneous)
	putMetric(measurement, "focal_return_energy_rate", pair.LeftEnergyRate(),
		data.UnitPerSecond, data.TimescaleInstantaneous)
	putMetric(measurement, "supported_return_count:measured", pair.LeftReturns(),
		data.UnitCount, data.TimescaleInstantaneous)
	putMetric(measurement, "supported_return_count:reference", pair.RightReturns(),
		data.UnitCount, data.TimescaleInstantaneous)

	putMetric(measurement, "shared_time", pair.SharedTime(),
		data.UnitSecond, data.TimescaleInstantaneous)
	putMetric(measurement, "overlap_density", pair.OverlapDensity(),
		data.UnitPerSecond, data.TimescaleInstantaneous)
	putMetric(measurement, "overlap_pair_count", pair.Support(),
		data.UnitCount, data.TimescaleInstantaneous)
	putMetric(measurement, "effective_sample_count", pair.Support(),
		data.UnitCount, data.TimescaleInstantaneous)

	if built.fisher.Ready() {
		putMetric(measurement, "correlation_p_value", built.fisher.PValue(),
			data.UnitDimensionless, data.TimescaleInstantaneous)
		putMetric(measurement, "correlation_standard_error_fisher",
			built.fisher.StandardError(),
			data.UnitDimensionless, data.TimescaleInstantaneous)
	}

	putMetric(measurement, "cohort_peer_count", cohort.Peers(),
		data.UnitCount, data.TimescaleInstantaneous)
	putMetric(measurement, "cohort_correlation_dispersion", cohort.Dispersion(),
		data.UnitNat, data.TimescaleInstantaneous)
	putMetric(measurement, "cohort_effective_peer_count", cohort.EffectivePeers(),
		data.UnitCount, data.TimescaleInstantaneous)

	relative := built.relativeEnergyOf()

	putMetric(measurement, "relative_return_energy", relative,
		data.UnitDimensionless, data.TimescaleInstantaneous)
	putMetric(measurement, "relative_cohort_return_energy", relative,
		data.UnitDimensionless, data.TimescaleInstantaneous)

	putMetric(measurement, "correlation_baseline", built.signedCorrelation.Baseline(),
		data.UnitDimensionless, data.TimescaleInstantaneous)
	putMetric(measurement, "correlation_divergence", built.signedCorrelation.Divergence(),
		data.UnitNat, data.TimescaleInstantaneous)
	putMetric(measurement, "correlation_zscore", built.signedCorrelation.ZScore(),
		data.UnitDimensionless, data.TimescaleInstantaneous)
	putMetric(measurement, "correlation_velocity", built.correlationVelocity.Rate(),
		data.UnitPerSecond, data.TimescaleInstantaneous)

	putMetric(measurement, "relative_return_energy_baseline",
		built.relativeEnergy.Baseline(),
		data.UnitDimensionless, data.TimescaleInstantaneous)
	putMetric(measurement, "relative_return_energy_divergence",
		built.relativeEnergy.Residual(),
		data.UnitDimensionless, data.TimescaleInstantaneous)
	putMetric(measurement, "relative_return_energy_zscore",
		built.relativeEnergy.ZScore(),
		data.UnitDimensionless, data.TimescaleInstantaneous)
	putMetric(measurement, "relative_return_energy_velocity",
		built.energyVelocity.Rate(),
		data.UnitPerSecond, data.TimescaleInstantaneous)

	// signed_correlation is the headline reading, so its Fisher-space
	// estimator supplies the quality facts Finalize derives maturity and SNR
	// from. The residual and the dispersion normalizing it share one space.
	if measurement.Metadata == nil {
		measurement.Metadata = map[string]float64{}
	}

	measurement.Metadata[data.MetadataSupport] = built.signedCorrelation.Count()

	if built.signedCorrelation.HasPrior() {
		measurement.Metadata[data.MetadataDivergence] =
			float64(built.signedCorrelation.Divergence())
		measurement.Metadata[data.MetadataNoiseVariance] =
			float64(built.signedCorrelation.NoiseVariance())
	}
}

func (ticker *Ticker) Close() error { return nil }

func putMetric(
	measurement *data.Measurement[float64],
	label string,
	raw nmtypes.Number,
	unit data.Unit,
	timescale data.Timescale,
) {
	measurement.PutMetric(data.Metric[float64]{
		Label:     label,
		Raw:       float64(raw),
		Unit:      unit,
		Timescale: timescale,
	})
}
