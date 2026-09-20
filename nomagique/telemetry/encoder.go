package telemetry

import (
	"fmt"

	flatbuffers "github.com/google/flatbuffers/go"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/telemetry/generated/telemetry"
)

/*
Encoder creates a closure that takes measurements and encodes them into a
telemetry Frame FlatBuffer for downstream consumption.
*/
func NewEncoder() types.Value[any, any] {
	return func(in any) any {
		var measurement *data.Measurement[float64]
		switch v := in.(type) {
		case *data.Measurement[float64]:
			measurement = v
		case data.Measurement[float64]:
			measurement = &v
		default:
			return in
		}

		if measurement == nil {
			return in
		}

		// Convert to FlatBuffer type
		m := &telemetry.MeasurementT{
			Id:           fmt.Sprint(measurement.ID),
			Source:       measurement.Source,
			Symbol:       measurement.Label,
			Tick:         measurement.SeqIdx,
			At:           measurement.At.UnixMilli(),
			ObservedFrom: measurement.From.UnixMilli(),
			Maturity:     measurement.Maturity,
			Snr:          measurement.SNR,
			SnrDefined:   measurement.SNRDefined,
		}

		frame := &telemetry.MeasurementsFrameT{
			Rows: []*telemetry.MeasurementT{m},
		}

		wrapper := &telemetry.FrameT{
			Type:  telemetry.FrameMeasurementsFrame,
			Value: frame,
		}

		builder := flatbuffers.NewBuilder(1024)
		offset := wrapper.Pack(builder)
		builder.Finish(offset)

		return builder.FinishedBytes()
	}
}
