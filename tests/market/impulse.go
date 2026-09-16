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
func ImpulseTape(symbol string, legs int) []*data.Measurement[float64] {
	tape := NewOpportunityTape(symbol, time.Unix(1700000000, 0), legs)
	frames := make([]*data.Measurement[float64], len(tape.Steps))

	for index, step := range tape.Steps {
		frame := data.NewMeasurement[float64]("frame", nil)
		frame.SeqIdx = int64(index + 1)
		frame.Label, frame.At = symbol, step.EventTime

		for _, name := range []string{"public", "direct", "inverse"} {
			observation := data.NewMeasurement[float64](name, nil)
			observation.Label, observation.At, observation.SeqIdx = symbol, step.EventTime, frame.SeqIdx
			observation.Provenance["owner"] = name
			observation.Maturity = 1
			value := step.ExecutableBid

			if name == "inverse" {
				value = -value
			}

			observation.Metrics["value"] = data.Metric[float64]{Label: "value", Raw: value}

			if name == "public" {
				quantity := decimal.NewFromInt64(int64(index%2 + 1))
				observation.Metadata["venue"] = "true"
				observation.Metadata["volume-unit"] = "base"
				observation.Provenance["channel"] = "trade"
				observation.Metrics["qty"] = data.Metric[float64]{Label: "qty", Raw: quantity.Float64(), Exact: quantity}
			}

			frame.Peers = append(frame.Peers, observation)
		}

		frames[index] = frame
	}
	return frames
}
