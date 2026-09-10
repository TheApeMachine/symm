package hindsight

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/hindsight/tables/tablestest"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/types"
)

func precursorPayload(value float64) []byte {
	at := time.Unix(1, 0)
	envelope := &types.Envelope{Key: "TEST/USD"}
	measurement := data.NewMeasurement[float64]("flow", "TEST/USD", "cvd", at, at)
	measurement.PutMetric(data.Metric[float64]{Label: "level", Raw: value})
	envelope.CVD = measurement

	return envelope.EncodePrecursor()
}

func statePayload(value float64) []byte {
	at := time.Unix(1, 0)
	envelope := &types.Envelope{Key: "TEST/USD"}
	measurement := data.NewMeasurement[float64]("flow", "TEST/USD", "cvd", at, at)
	measurement.PutMetric(data.Metric[float64]{Label: "level", Raw: value})
	envelope.CVD = measurement

	return envelope.EncodeBytes()
}

func TestTapeMeasurementsFrom(t *testing.T) {
	Convey("Confirmed excursions are projected from one precursor scan", t, func() {
		series := excursionSeries()
		catalog := tablestest.New(t)
		writer := tables.NewWriter(catalog)
		omitted := series[150]
		omittedIdentity := tables.EnvelopeRefRow{
			Run:      string(omitted.Capture.Run),
			Sequence: int64(omitted.Capture.Sequence),
			Ordinal:  int64(omitted.Ordinal),
		}

		for _, observation := range series {
			identity := tables.EnvelopeRefRow{
				Run:      string(observation.Capture.Run),
				Sequence: int64(observation.Capture.Sequence),
				Ordinal:  int64(observation.Ordinal),
			}

			if identity == omittedIdentity {
				writer.AddWitness(tables.WitnessRow{
					Run: identity.Run, Envelope: identity, ArtifactKind: "state",
					Boundary: "after-logic", Payload: statePayload(observation.Bid),
				})
				continue
			}
			writer.AddWitness(tables.WitnessRow{
				Run: identity.Run, Envelope: identity, ArtifactKind: "precursor",
				Boundary: "after-logic", Payload: precursorPayload(observation.Bid),
			})
		}
		writer.AddWitness(tables.WitnessRow{
			Run: "other", Envelope: tables.EnvelopeRefRow{Run: "other", Sequence: 1},
			ArtifactKind: "precursor", Boundary: "after-logic",
			Payload: precursorPayload(999),
		})
		So(writer.Commit(t.Context()), ShouldBeNil)

		tape := Query(Excursions, catalog, "run-test", DefaultDiscoveryPolicy())
		legs := tape.MeasurementsFrom(series)
		So(tape.Error(), ShouldBeNil)
		So(len(legs), ShouldBeGreaterThan, 0)
		So(len(legs[0]), ShouldBeGreaterThan, 0)
		So(len(legs[0][0]), ShouldBeGreaterThan, 0)
		So(legs[0][0][0].Metrics["level"].Raw, ShouldNotEqual, 999)
		So(legs[0][0][0].Provenance["moment"], ShouldBeIn, "enter", "hold", "exit", "wait")

		found := false

		for _, leg := range legs {
			for _, frame := range leg {
				for _, measurement := range frame {
					if measurement.Metrics["level"].Raw == omitted.Bid {
						found = true
					}
				}
			}
		}
		So(found, ShouldBeTrue)
	})

	Convey("An unconfirmed live move is admitted only when asked for live", t, func() {
		series := excursionSeries()
		catalog := tablestest.New(t)
		writer := tables.NewWriter(catalog)

		for _, observation := range series {
			identity := tables.EnvelopeRefRow{
				Run:      string(observation.Capture.Run),
				Sequence: int64(observation.Capture.Sequence),
				Ordinal:  int64(observation.Ordinal),
			}
			writer.AddWitness(tables.WitnessRow{
				Run: identity.Run, Envelope: identity, ArtifactKind: "precursor",
				Boundary: "after-logic", Payload: precursorPayload(observation.Bid),
			})
		}
		So(writer.Commit(t.Context()), ShouldBeNil)

		confirmed := Query(Excursions, catalog, "run-test", DefaultDiscoveryPolicy())
		historical := confirmed.MeasurementsFrom(series)
		So(confirmed.Error(), ShouldBeNil)

		live := Query(Excursions, catalog, "run-test", DefaultDiscoveryPolicy())
		developing := live.MeasurementsLive(series)
		So(live.Error(), ShouldBeNil)
		So(len(developing), ShouldBeGreaterThanOrEqualTo, len(historical))
	})
}
