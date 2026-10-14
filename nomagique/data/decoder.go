package data

import (
	"time"

	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

/*
FromWire decodes a wire.MeasurementT into a domain Measurement.
*/
func FromWire(row *wire.MeasurementT) *Measurement {
	if row == nil {
		return nil
	}

	meas := NewMeasurement(
		0,
		row.Symbol,
		row.Source,
		time.Unix(0, row.At).UTC(),
		time.Unix(0, row.ObservedFrom).UTC(),
	)

	meas.maturity = row.Maturity

	for _, metric := range row.Metrics {
		if metric == nil {
			continue
		}
		unit, timescale := GetDimension(row.Source, metric.Name)
		item := NewMetric(metric.Name, metric.Raw, unit, timescale)
		item.snm = metric.Snm
		meas.metrics = append(meas.metrics, &MetricEntry{
			Key:    metric.Name,
			Metric: item,
		})
	}

	for _, peer := range row.Peers {
		if peer == nil {
			continue
		}
		if wirePeer := FromWire(peer); wirePeer != nil {
			meas.peers = append(meas.peers, wirePeer)
		}
	}

	meas.finalize()
	return meas
}
