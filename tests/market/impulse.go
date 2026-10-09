package market

import (
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
ImpulseTape turns the shared multi-leg opportunity fixture into complete
workspace frames. Opposed signals observe the same tape; alternating exact
trade sizes make event count different from the volume clock. Every frame
owns its publications, as persisted workspace output does.
*/
func ImpulseTape(symbol string, legs int) []*data.Measurement {
	tape := NewOpportunityTape(symbol, time.Unix(1700000000, 0), legs)
	frames := make([]*data.Measurement, len(tape.Steps))

	for index, step := range tape.Steps {
		seqIdx := int64(index + 1)
		peers := make([]*data.Measurement, 0, 3)

		for _, name := range []string{"public", "direct", "inverse"} {
			value := step.ExecutableBid

			if name == "inverse" {
				value = -value
			}

			var metadata []*data.StringEntry
			var metrics []*data.Metric

			valueMetric := data.NewMetric(
				"value",
				value,
				data.UnitPrice,
				data.TimescaleInstantaneous,
			)
			metrics = append(metrics, valueMetric)

			if name == "public" {
				quantity := decimal.NewFromInt64(int64(index%2 + 1))
				metadata = append(
					metadata,
					&data.StringEntry{Key: "venue", Value: "true"},
					&data.StringEntry{Key: "volume-unit", Value: "base"},
					&data.StringEntry{Key: "channel", Value: "trade"},
				)

				qtyMetric := data.NewExactMetric(
					"qty",
					quantity,
					data.UnitQuantity,
					data.TimescaleInstantaneous,
				)
				metrics = append(metrics, qtyMetric)
			}

			observation := data.NewMeasurement(
				1,
				symbol,
				name,
				seqIdx,
				seqIdx,
				metadata...,
			)
			observation.At = step.EventTime
			observation.From = step.EventTime
			observation.Write(metrics...)

			peers = append(peers, observation)
		}

		frame := data.NewMeasurement(
			1,
			symbol,
			"frame",
			seqIdx,
			seqIdx,
		)
		frame.Peers(peers...)
		frame.At = step.EventTime
		frame.From = step.EventTime
		frame.Write()

		frames[index] = frame
	}

	return frames
}
