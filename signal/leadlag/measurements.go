package leadlag

import (
	"time"

	"github.com/google/uuid"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

func (signal *Signal) measurement(
	symbol string,
	anchor string,
	at time.Time,
	anchorPath nmtypes.Frame,
	followerPath nmtypes.Frame,
	output nmtypes.Frame,
	measured bool,
) *nmtypes.Measurement {
	measurement := signal.baseMeasurement(symbol, anchor, at, followerPath)
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
		nmtypes.NewMetric(string(types.MetricCorrelation), metricValue(output, correlation.SymbolLeadLagCorrelation, measured), dimensionless),
		nmtypes.NewMetric(string(types.MetricSignedCorrelation), metricValue(output, correlation.SymbolSignedCorrelation, measured), dimensionless),
		nmtypes.NewMetric(string(types.MetricSignedContempCorrelation), metricValue(output, correlation.SymbolContempCorrelation, measured), dimensionless),
		nmtypes.NewMetric(string(types.MetricSignedLagCorrelation), metricValue(output, correlation.SymbolLagCorrelation, measured), dimensionless),
		nmtypes.NewMetric(string(types.MetricLagFraction), metricValue(output, correlation.SymbolLagFraction, measured), dimensionless),
		nmtypes.NewMetric(string(types.MetricSampleCount), metricValue(output, correlation.SymbolLeadLagSampleCount, measured), nmtypes.Descriptor{
			Unit: nmtypes.UnitCount, Timescale: nmtypes.TimescaleInstantaneous,
		}),
		nmtypes.NewNormalizedMetric(string(types.MetricInefficient), metricValue(output, correlation.SymbolInefficiency, measured), metricValue(output, correlation.SymbolInefficiency, measured), dimensionless),
		nmtypes.NewNormalizedMetric(string(types.MetricSync), metricValue(output, correlation.SymbolSync, measured), metricValue(output, correlation.SymbolSync, measured), dimensionless),
		nmtypes.NewNormalizedMetric(string(types.MetricDecoupled), metricValue(output, correlation.SymbolDecoupled, measured), metricValue(output, correlation.SymbolDecoupled, measured), dimensionless),
		nmtypes.NewNormalizedMetric(string(types.MetricStall), metricValue(output, correlation.SymbolStall, measured), metricValue(output, correlation.SymbolStall, measured), dimensionless),
		nmtypes.NewMetric(string(types.MetricStrength), metricValue(output, correlation.SymbolLeadLagStrength, measured), dimensionless),
		nmtypes.NewMetric(string(types.MetricSignedLagDirection), metricValue(output, correlation.SymbolLagDirection, measured), dimensionless),
	)

	separation := metricValue(output, correlation.SymbolLeadLagSeparation, measured)

	measurement.StampQuality(separation, pathSupport(followerPath))

	return measurement
}

/*
neutralMeasurement is the always-emitted lead-lag row for a symbol that has no
anchor yet. It reports the follower's own last price and observation count and
nothing else, with zero separation and low maturity, so downstream consumers
see the symbol immediately instead of waiting for a peer to appear.
*/
func (signal *Signal) neutralMeasurement(
	symbol string,
	at time.Time,
	price float64,
) *nmtypes.Measurement {
	path, _ := signal.number.Project(symbol)
	measurement := signal.baseMeasurement(symbol, "", at, path)
	dimensionless := nmtypes.Descriptor{
		Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous,
	}
	measurement.AddMetrics(
		nmtypes.NewMetric(string(types.MetricLastPrice), price, dimensionless),
		nmtypes.NewMetric(string(types.MetricSampleCount), pathSupport(path), nmtypes.Descriptor{
			Unit: nmtypes.UnitCount, Timescale: nmtypes.TimescaleInstantaneous,
		}),
	)
	measurement.StampQuality(0, pathSupport(path))

	return measurement
}

func (signal *Signal) baseMeasurement(
	symbol string,
	peer string,
	at time.Time,
	followerPath nmtypes.Frame,
) *nmtypes.Measurement {
	measurement := nmtypes.NewMeasurement(
		uuid.NewString(),
		signal.Name(),
		at.UnixNano(),
		at.UnixNano(),
	)
	measurement.Peer = peer
	from, _, _ := pathBoundary(followerPath)

	if !from.IsZero() {
		measurement.ObservedFrom = from
	}

	return measurement
}

func metricValue(
	frame nmtypes.Frame,
	symbol nmtypes.Symbol,
	measured bool,
) float64 {
	if !measured {
		return 0
	}

	value, found := frame.Get(symbol)

	if found {
		return value
	}

	return 0
}

func pathSupport(path nmtypes.Frame) float64 {
	value, found := path.Get(nmtypes.SampleCount)

	if found {
		return value
	}

	return 0
}

func leadLagObservedFrom(anchorPath nmtypes.Frame, followerPath nmtypes.Frame) time.Time {
	anchorFrom, _, _ := pathBoundary(anchorPath)
	followerFrom, _, _ := pathBoundary(followerPath)

	if followerFrom.After(anchorFrom) {
		return followerFrom
	}

	return anchorFrom
}

func pathBoundary(path nmtypes.Frame) (time.Time, time.Time, float64) {
	count, _ := path.Get(nmtypes.SampleCount)
	firstTimestamp, _, hasFirst := temporal.PathSample(&path, 0)
	lastTimestamp, lastValue, hasLast := temporal.PathSample(&path, int(count)-1)

	if !hasFirst || !hasLast {
		return time.Time{}, time.Time{}, 0
	}

	return time.Unix(0, firstTimestamp), time.Unix(0, lastTimestamp), lastValue
}
