package types

import (
	"fmt"
	"strconv"
	"time"

	flatbuffers "github.com/google/flatbuffers/go"
	"github.com/theapemachine/symm/nomagique/data"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

func EncodeMeasurementsFrame(measurements []*data.Measurement[float64]) []byte {
	rows := make([]*wire.MeasurementT, 0, len(measurements))

	for _, m := range measurements {
		if m == nil {
			continue
		}

		metrics := make([]*wire.MetricT, 0, len(m.Metrics))
		for _, metric := range m.Metrics {
			met := &wire.MetricT{
				Name: metric.Label,
				Raw:  metric.Raw,
				Unit: string(metric.Unit),
			}

			if metric.Normalized != nil {
				met.Normalized = *metric.Normalized
				met.HasNormalized = true
			}

			metrics = append(metrics, met)
		}

		metadata := make([]*wire.NamedNumberT, 0, len(m.Metadata))
		for k, v := range m.Metadata {
			if fval, err := strconv.ParseFloat(v, 64); err == nil {
				metadata = append(metadata, &wire.NamedNumberT{
					Name:  k,
					Value: fval,
				})
			}
		}

		rows = append(rows, &wire.MeasurementT{
			Source:       m.Source,
			Symbol:       m.Label,
			Tick:         m.SeqIdx,
			At:           m.At.UnixNano(),
			ObservedFrom: m.From.UnixNano(),
			Maturity:     m.Maturity,
			Snr:          m.SNR,
			SnrDefined:   m.SNRDefined,
			Metrics:      metrics,
			Metadata:     metadata,
		})
	}

	if len(rows) == 0 {
		return nil
	}

	frame := &wire.MeasurementsFrameT{
		Rows: rows,
	}

	builder := flatbuffers.NewBuilder(1024)
	offset := frame.Pack(builder)
	builder.Finish(offset)

	return builder.FinishedBytes()
}

func MeasurementsFromState(payload []byte) (measurements []*data.Measurement[float64], err error) {
	if len(payload) == 0 {
		return nil, nil
	}

	defer func() {
		if invalid := recover(); invalid != nil {
			measurements, err = nil, fmt.Errorf("measurements: malformed state: %v", invalid)
		}
	}()

	frame := wire.GetRootAsMeasurementsFrame(payload, 0)
	count := frame.RowsLength()
	measurements = make([]*data.Measurement[float64], 0, count)

	for i := 0; i < count; i++ {
		row := new(wire.Measurement)
		if !frame.Rows(row, i) {
			continue
		}

		m := data.NewMeasurement[float64](string(row.Source()), nil)
		m.Label = string(row.Symbol())
		m.SeqIdx = row.Tick()
		m.At = time.Unix(0, row.At())
		m.From = time.Unix(0, row.ObservedFrom())
		m.Maturity = row.Maturity()
		m.SNR = row.Snr()
		m.SNRDefined = row.SnrDefined()

		metricsCount := row.MetricsLength()
		for j := 0; j < metricsCount; j++ {
			metric := new(wire.Metric)
			if !row.Metrics(metric, j) {
				continue
			}

			decoded := data.Metric[float64]{
				Label: string(metric.Name()),
				Raw:   metric.Raw(),
				Unit:  data.Unit(string(metric.Unit())),
			}

			if metric.HasNormalized() {
				norm := metric.Normalized()
				decoded.Normalized = &norm
			}

			m.Metrics[decoded.Label] = decoded
		}

		metadataCount := row.MetadataLength()
		for j := 0; j < metadataCount; j++ {
			item := new(wire.NamedNumber)
			if !row.Metadata(item, j) {
				continue
			}

			if m.Metadata == nil {
				m.Metadata = make(map[string]string)
			}

			m.Metadata[string(item.Name())] = strconv.FormatFloat(item.Value(), 'f', -1, 64)
		}

		measurements = append(measurements, m)
	}

	return measurements, nil
}
