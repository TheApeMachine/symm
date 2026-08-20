package leadlag

import (
	"math"
	"time"

	"github.com/google/uuid"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

type evidence struct {
	correlation       float64
	signed            float64
	signedContemporal float64
	signedLag         float64
	lagFraction       float64
	inefficient       float64
	sync              float64
	decoupled         float64
	stall             float64
	strength          float64
}

func (signal *Signal) measurement(symbol string, at time.Time) *nmtypes.Measurement {
	features := signal.section.Features(symbol)
	weights := leadLagEvidence(features, signal.section.maxLagBars(features.SampleCount))
	measurement := nmtypes.NewMeasurement(uuid.NewString(), signal.Name(), at.UnixNano(), at.UnixNano())
	measurement.Peer = signal.section.AnchorSymbol()
	measurement.ObservedFrom = features.ObservedFrom
	measurement.PeerAt = features.PeerAt
	measurement.PeerObservedFrom = features.PeerFrom

	if features.IsAnchor || signal.section.AnchorSymbol() == "" {
		measurement.Peer = ""
	}

	dimensionless := nmtypes.Descriptor{
		Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous,
	}
	measurement.AddMetrics(
		nmtypes.NewMetric(string(types.MetricLastPrice), features.Price, dimensionless),
		nmtypes.NewMetric(string(types.MetricPeerLastPrice), features.PeerPrice, dimensionless),
		nmtypes.NewMetric(string(types.MetricCorrelation), weights.correlation, dimensionless),
		nmtypes.NewMetric(string(types.MetricSignedCorrelation), weights.signed, dimensionless),
		nmtypes.NewMetric(string(types.MetricSignedContempCorrelation), weights.signedContemporal, dimensionless),
		nmtypes.NewMetric(string(types.MetricSignedLagCorrelation), weights.signedLag, dimensionless),
		nmtypes.NewMetric(string(types.MetricLagFraction), weights.lagFraction, dimensionless),
		nmtypes.NewMetric(string(types.MetricSampleCount), float64(features.SampleCount), nmtypes.Descriptor{
			Unit: nmtypes.UnitCount, Timescale: nmtypes.TimescaleInstantaneous,
		}),
		normalized(string(types.MetricInefficient), weights.inefficient, dimensionless),
		normalized(string(types.MetricSync), weights.sync, dimensionless),
		normalized(string(types.MetricDecoupled), weights.decoupled, dimensionless),
		normalized(string(types.MetricStall), weights.stall, dimensionless),
		nmtypes.NewMetric(string(types.MetricStrength), weights.strength, dimensionless),
		nmtypes.NewMetric(string(types.MetricSignedLagDirection), boolFloat(weights.signedLag != 0), dimensionless),
	)

	return measurement
}

func leadLagEvidence(features LagFeatures, maxLagBars int) evidence {
	result := evidence{signedContemporal: boundedSigned(features.ContempCorr)}
	searches := max(maxLagBars, 1)
	support := max(features.SampleCount, 1)
	threshold := math.Min(1, math.Sqrt(2*math.Log(float64(searches+1))/float64(support)))

	if features.LagOK && math.Abs(features.LagCorr) >= threshold {
		result.signedLag = boundedSigned(features.LagCorr)
		result.lagFraction = math.Min(1, math.Abs(float64(features.LagBars))/float64(searches))
	}

	result.signed = result.signedContemporal

	if math.Abs(result.signedLag) > math.Abs(result.signed) {
		result.signed = result.signedLag
	}

	result.correlation = math.Abs(result.signed)
	shortWindow := max(resolvedShortWindow(features.SampleCount), 1)
	sampleSupport := math.Min(1, float64(features.SampleCount)/float64(shortWindow))
	anchorActive := boolFloat(features.MoveReady)
	moving := boolFloat(features.MoveMoved)
	stallMargin := bounded(features.StallMargin)
	relationGap := 1 - result.correlation
	lagGap := 1 - result.lagFraction

	result.inefficient = bounded(sampleSupport * anchorActive * moving *
		math.Abs(result.signedLag) * result.lagFraction * (1 - stallMargin))
	result.sync = bounded(sampleSupport * anchorActive * moving *
		math.Abs(result.signedContemporal) * lagGap * (1 - stallMargin))
	result.decoupled = bounded(sampleSupport * anchorActive * relationGap * (1 - stallMargin))
	result.stall = bounded(sampleSupport * anchorActive * relationGap * lagGap * stallMargin)
	result.strength = max(result.inefficient, result.sync, result.decoupled, result.stall)

	return result
}

func normalized(name string, value float64, descriptor nmtypes.Descriptor) *nmtypes.Metric[float64] {
	return nmtypes.NewNormalizedMetric(name, value, value, descriptor)
}

func bounded(value float64) float64 {
	return max(0, min(1, value))
}

func boundedSigned(value float64) float64 {
	return max(-1, min(1, value))
}

func boolFloat(value bool) float64 {
	if value {
		return 1
	}

	return 0
}
