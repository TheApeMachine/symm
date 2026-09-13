package sentiment

import (
	"context"
	"unsafe"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/crosssection"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Ticker is the cross-sectional change-breadth instrument. It holds no state
and no logic of its own: its entire behavior is one nomagique pipeline over
the measurement itself — every stage writes its facts into the measurement
where it computes them, and the workload's register owns the measurement's
lifetime.
*/
type Ticker struct {
	*runtime.System
	pipeline core.Primitive
	ID       int
}

func NewTicker(ctx context.Context) *Ticker {
	prices := store.NewLatest[string, float64]()
	changes := store.NewLatest[string, data.CrossMember]()

	return &Ticker{
		System: runtime.NewSystem(ctx, "sentiment:ticker"),
		pipeline: nomagique.NewNumber(
			data.NewMetricGate("last"),
			crosssection.NewUpdateMember("last", prices, changes),
			crosssection.NewStampPeers(changes),
			crosssection.NewChangeCounts(),
			crosssection.NewChangeMedian(),
			crosssection.NewChangeBaseline(),
			data.NewFinalizer[float64](),
		),
	}
}

/*
Step supplies the arriving measurement to the pipeline and returns it: the
measurement is the pipeline's state, enriched in place.
*/
func (ticker *Ticker) Step(m *data.Measurement[float64]) *data.Measurement[float64] {
	return data.Read[*data.Measurement[float64]](ticker.pipeline.Next(transport.NewOne(unsafe.Pointer(&m)).Next(nil)))
}

/*
Register returns the pre-allocated measurement every sentiment tick flows
through: the feed's last price plus every cross-section fact the pipeline can
write, declared, none valued.
*/
func (ticker *Ticker) Register() *data.Measurement[float64] {
	return data.NewMeasurement("sentiment", map[string]data.Metric[float64]{
		"last": data.NewMetric[float64](
			"last", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"valid_member_count": data.NewMetric[float64](
			"valid_member_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"member_count": data.NewMetric[float64](
			"member_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"excluded_member_count": data.NewMetric[float64](
			"excluded_member_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"positive_count": data.NewMetric[float64](
			"positive_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"negative_count": data.NewMetric[float64](
			"negative_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"zero_count": data.NewMetric[float64](
			"zero_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"extreme_tie_count": data.NewMetric[float64](
			"extreme_tie_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"max_age": data.NewMetric[float64](
			"max_age", data.UnitSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"mean_age": data.NewMetric[float64](
			"mean_age", data.UnitSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"median_age": data.NewMetric[float64](
			"median_age", data.UnitSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"median_from_age": data.NewMetric[float64](
			"median_from_age", data.UnitSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"focal_age": data.NewMetric[float64](
			"focal_age", data.UnitSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"focal_from_age": data.NewMetric[float64](
			"focal_from_age", data.UnitSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"signed_median": data.NewMetric[float64](
			"signed_median", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"mean_absolute": data.NewMetric[float64](
			"mean_absolute", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"median_absolute": data.NewMetric[float64](
			"median_absolute", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"mad": data.NewMetric[float64](
			"mad", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"magnitude_mad": data.NewMetric[float64](
			"magnitude_mad", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"interquartile_range": data.NewMetric[float64](
			"interquartile_range", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"rms": data.NewMetric[float64](
			"rms", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"extreme_magnitude": data.NewMetric[float64](
			"extreme_magnitude", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"extreme_signed": data.NewMetric[float64](
			"extreme_signed", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"peer_median_absolute": data.NewMetric[float64](
			"peer_median_absolute", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"peer_mad": data.NewMetric[float64](
			"peer_mad", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"signed_fraction": data.NewMetric[float64](
			"signed_fraction", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"signed_fraction_baseline": data.NewMetric[float64](
			"signed_fraction_baseline", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"signed_fraction_divergence": data.NewMetric[float64](
			"signed_fraction_divergence", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"signed_fraction_zscore": data.NewMetric[float64](
			"signed_fraction_zscore", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"signed_fraction_velocity": data.NewMetric[float64](
			"signed_fraction_velocity", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"median_absolute_baseline": data.NewMetric[float64](
			"median_absolute_baseline", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"median_absolute_divergence": data.NewMetric[float64](
			"median_absolute_divergence", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"median_absolute_zscore": data.NewMetric[float64](
			"median_absolute_zscore", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"median_absolute_velocity": data.NewMetric[float64](
			"median_absolute_velocity", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"iqr": data.NewMetric[float64](
			"iqr", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"iqr_baseline": data.NewMetric[float64](
			"iqr_baseline", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"iqr_divergence": data.NewMetric[float64](
			"iqr_divergence", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"iqr_zscore": data.NewMetric[float64](
			"iqr_zscore", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"iqr_velocity": data.NewMetric[float64](
			"iqr_velocity", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"extreme_ratio": data.NewMetric[float64](
			"extreme_ratio", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"extreme_ratio_baseline": data.NewMetric[float64](
			"extreme_ratio_baseline", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"extreme_ratio_divergence": data.NewMetric[float64](
			"extreme_ratio_divergence", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"extreme_ratio_zscore": data.NewMetric[float64](
			"extreme_ratio_zscore", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"extreme_ratio_velocity": data.NewMetric[float64](
			"extreme_ratio_velocity", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"extreme_share": data.NewMetric[float64](
			"extreme_share", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"extreme_share_baseline": data.NewMetric[float64](
			"extreme_share_baseline", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"extreme_share_divergence": data.NewMetric[float64](
			"extreme_share_divergence", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"extreme_share_zscore": data.NewMetric[float64](
			"extreme_share_zscore", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"extreme_share_velocity": data.NewMetric[float64](
			"extreme_share_velocity", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
	})
}
