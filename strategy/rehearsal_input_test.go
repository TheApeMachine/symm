package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestRehearsalReadInputs(t *testing.T) {
	Convey("A captured precursor joins the preceding quote, never a later price", t, func() {
		learner, _ := learningFixture(t)
		rehearsal := learner.Rehearsal
		before := rehearsalObservation(1, 0, "BTC/USD", 99, 100, 99.5)
		current := rehearsalObservation(2, 0, "BTC/USD", 0, 0, 0)
		future := rehearsalObservation(3, 0, "BTC/USD", 199, 200, 199.5)
		before.Capture.Run, current.Capture.Run, future.Capture.Run = learner.run, learner.run, learner.run
		writer := tables.NewWriter(learner.catalog)

		for _, observation := range []hindsight.Observation{before, current, future} {
			writer.AddCapture(tables.CaptureRow{Run: string(learner.run), Sequence: int64(observation.Capture.Sequence),
				Stream: "spot", StreamEpoch: 1, ReceivedAt: observation.ReceivedAt})
		}
		other := rehearsalObservation(1, 0, "ETH/USD", 19, 20, 19.5)
		rehearsal.observations[other.Symbol] = []hindsight.Observation{other}
		envelope := rehearsalEnvelope(current)
		envelope.Derivatives = data.NewMeasurement[float64]("other-symbol", other.Symbol, "derivatives", current.ReceivedAt, before.ReceivedAt)
		envelope.Derivatives.PutMetric(data.Metric[float64]{Label: "basis", Raw: 1})
		writer.AddWitness(tables.WitnessRow{Run: string(learner.run), ArtifactKind: "precursor",
			Envelope: tables.EnvelopeRefRow{Run: string(learner.run), Sequence: 2}, Payload: envelope.EncodePrecursor()})
		So(writer.Commit(t.Context()), ShouldBeNil)
		rehearsal.observations["BTC/USD"] = []hindsight.Observation{before, future}
		rehearsal.lastSequence = 3
		So(rehearsal.readInputs(t.Context()), ShouldBeNil)
		So(len(rehearsal.observations["BTC/USD"]), ShouldEqual, 3)
		So(rehearsal.observations["BTC/USD"][1].Bid, ShouldEqual, before.Bid)
		So(len(rehearsal.inputs), ShouldEqual, 1)
		So(len(rehearsal.observations[other.Symbol]), ShouldEqual, 2)
		So(rehearsal.observations[other.Symbol][1].Bid, ShouldEqual, other.Bid)
		So(rehearsal.readInputs(t.Context()), ShouldBeNil)
		So(len(rehearsal.observations["BTC/USD"]), ShouldEqual, 3)
	})
}
