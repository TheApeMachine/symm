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
		frame := data.NewMeasurement("frame", nil)
		frame.SeqIdx = int64(index + 1)
		frame.Label, frame.At = symbol, step.EventTime

		for _, name := range []string{"public", "direct", "inverse"} {
			observation := data.NewMeasurement(name, nil)
			observation.Label, observation.At, observation.SeqIdx = symbol, step.EventTime, frame.SeqIdx
			observation.SetProvenance("owner", name)
			observation.Maturity = 1
			value := step.ExecutableBid

			if name == "inverse" {
				value = -value
			}

			observation.SetMetric("value", data.Metric{Label: "value", Raw: value, Standardized: &value})

			if name == "public" {
				quantity := decimal.NewFromInt64(int64(index%2 + 1))
				observation.SetMetadata("venue", "true")
				observation.SetMetadata("volume-unit", "base")
				observation.SetProvenance("channel", "trade")
				qVal := quantity.Float64()
				observation.SetMetric("qty", data.Metric{Label: "qty", Raw: qVal, Standardized: &qVal, Exact: quantity})
			}

			frame.Peers = append(frame.Peers, observation)
		}

		frames[index] = frame
	}
	return frames
}
