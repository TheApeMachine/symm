package leadlag

import (
	"time"

	"github.com/google/uuid"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

func (signal *Signal) measurement(
	symbol string,
	anchor string,
	at time.Time,
	anchorPath nomagique.Frame,
	followerPath nomagique.Frame,
	output nomagique.Frame,
) *nmtypes.Measurement {
	measurement := nmtypes.NewMeasurement(
		uuid.NewString(),
		signal.Name(),
		at.UnixNano(),
		at.UnixNano(),
	)
	measurement.Peer = anchor
	measurement.ObservedFrom = leadLagObservedFrom(anchorPath, followerPath)
	peerFrom, peerAt, peerPrice := pathBoundary(anchorPath)
	_, _, price := pathBoundary(followerPath)
	measurement.PeerObservedFrom = peerFrom
	measurement.PeerAt = peerAt
	dimensionless := nmtypes.Descriptor{
		Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous,
	}
	measurement.AddMetrics(
		nmtypes.NewMetric(string(types.MetricLastPrice), price, dimensionless),
		nmtypes.NewMetric(string(types.MetricPeerLastPrice), peerPrice, dimensionless),
		nmtypes.NewMetric(string(types.MetricCorrelation), output.MustGet(algo.SymbolLeadLagCorrelation), dimensionless),
		nmtypes.NewMetric(string(types.MetricSignedCorrelation), output.MustGet(algo.SymbolSignedCorrelation), dimensionless),
		nmtypes.NewMetric(string(types.MetricSignedContempCorrelation), output.MustGet(equation.SymbolContempCorrelation), dimensionless),
		nmtypes.NewMetric(string(types.MetricSignedLagCorrelation), output.MustGet(equation.SymbolLagCorrelation), dimensionless),
		nmtypes.NewMetric(string(types.MetricLagFraction), output.MustGet(equation.SymbolLagFraction), dimensionless),
		nmtypes.NewMetric(string(types.MetricSampleCount), output.MustGet(equation.SymbolLeadLagSampleCount), nmtypes.Descriptor{
			Unit: nmtypes.UnitCount, Timescale: nmtypes.TimescaleInstantaneous,
		}),
		nmtypes.NewNormalizedMetric(string(types.MetricInefficient), output.MustGet(algo.SymbolInefficiency), output.MustGet(algo.SymbolInefficiency), dimensionless),
		nmtypes.NewNormalizedMetric(string(types.MetricSync), output.MustGet(algo.SymbolSync), output.MustGet(algo.SymbolSync), dimensionless),
		nmtypes.NewNormalizedMetric(string(types.MetricDecoupled), output.MustGet(algo.SymbolDecoupled), output.MustGet(algo.SymbolDecoupled), dimensionless),
		nmtypes.NewNormalizedMetric(string(types.MetricStall), output.MustGet(algo.SymbolStall), output.MustGet(algo.SymbolStall), dimensionless),
		nmtypes.NewMetric(string(types.MetricStrength), output.MustGet(algo.SymbolLeadLagStrength), dimensionless),
		nmtypes.NewMetric(string(types.MetricSignedLagDirection), output.MustGet(algo.SymbolLagDirection), dimensionless),
		nmtypes.NewMetric(string(types.MetricHypothesisSeparation), output.MustGet(algo.SymbolLeadLagSeparation), dimensionless),
	)

	return measurement
}

func leadLagObservedFrom(anchorPath nomagique.Frame, followerPath nomagique.Frame) time.Time {
	anchorFrom, _, _ := pathBoundary(anchorPath)
	followerFrom, _, _ := pathBoundary(followerPath)

	if followerFrom.After(anchorFrom) {
		return followerFrom
	}

	return anchorFrom
}

func pathBoundary(path nomagique.Frame) (time.Time, time.Time, float64) {
	count, _ := path.Get(nomagique.SampleCount)
	firstTimestamp, _, hasFirst := temporal.PathSample(&path, 0)
	lastTimestamp, lastValue, hasLast := temporal.PathSample(&path, int(count)-1)

	if !hasFirst || !hasLast {
		return time.Time{}, time.Time{}, 0
	}

	return time.Unix(0, firstTimestamp), time.Unix(0, lastTimestamp), lastValue
}
