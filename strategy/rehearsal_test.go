package strategy

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/nomagique/learning/associative/prior"
)

func rehearsalObservation(
	sequence, ordinal uint64,
	symbol string,
	bid, ask, last float64,
) hindsight.Observation {
	at := time.Unix(1700000000, 0).Add(time.Duration(sequence) * time.Second)

	return hindsight.Observation{
		Domain:  "spot",
		Capture: hindsight.CaptureIdentity{Run: "rehearsal", Sequence: hindsight.CaptureSequence(sequence), Stream: "spot", StreamEpoch: 1},
		Ordinal: ordinal,
		Symbol:  symbol,
		Kind:    "ticker",
		VenueAt: at,
		HasBid:  bid > 0,
		Bid:     bid,
		HasAsk:  ask > 0,
		Ask:     ask,
		HasLast: last > 0,
		Last:    last,
	}
}

func TestMeasurementFromObservation(t *testing.T) {
	Convey("A historical observation projects only the facts it defines", t, func() {
		measurement := measurementFromObservation(rehearsalObservation(1, 0, "BTC/USD", 99, 101, 100))

		So(measurement.Source, ShouldEqual, rawMarketSource)
		So(measurement.Label, ShouldEqual, "BTC/USD")
		So(measurement.Metrics["bid"].Raw, ShouldEqual, 99)
		So(measurement.Metrics["ask"].Raw, ShouldEqual, 101)
		So(measurement.Metrics["last"].Raw, ShouldEqual, 100)
		So(measurement.Metrics["spread"].Raw, ShouldBeGreaterThan, 0)
		So(measurement.Metrics["depth"].Raw, ShouldEqual, 0)

		space := grid.NewSpace()
		So(space.Step([]*data.Measurement[float64]{measurement}), ShouldBeNil)
		So(measurement.Err, ShouldBeNil)
		So(space.Version, ShouldEqual, 1)
	})
}

func TestGroupObservations(t *testing.T) {
	Convey("Observations are grouped, sorted by capture order and spot only", t, func() {
		grouped := groupObservations([]hindsight.Observation{
			rehearsalObservation(2, 0, "BTC/USD", 99, 101, 100),
			rehearsalObservation(1, 0, "BTC/USD", 98, 100, 99),
			rehearsalObservation(3, 0, "BTC/USD", 100, 102, 101),
			{Domain: "futures", Symbol: "BTC/USD", Capture: hindsight.CaptureIdentity{Run: "rehearsal", Sequence: 4, Stream: "futures", StreamEpoch: 1}},
		})

		So(len(grouped), ShouldEqual, 1)
		So(len(grouped["BTC/USD"]), ShouldEqual, 3)
		So(grouped["BTC/USD"][0].Capture.Sequence, ShouldEqual, hindsight.CaptureSequence(1))
		So(grouped["BTC/USD"][2].Capture.Sequence, ShouldEqual, hindsight.CaptureSequence(3))
	})
}

func TestRehearsalGrade(t *testing.T) {
	Convey("Waiting and entering are graded symmetrically around the same excursion", t, func() {
		rehearsal := &Rehearsal{}
		anchor := rehearsalObservation(1, 0, "BTC/USD", 99, 101, 100)
		episode := hindsight.Episode{
			Symbol:               "BTC/USD",
			Kind:                 hindsight.EpisodeUpwardExcursion,
			ObservedExcursion:    0.10,
			HasObservedExcursion: true,
		}
		selected := prior.Reading{Authority: 0.5}

		entered, authority := rehearsal.grade(episode, Action{Kind: "enter"}, selected, anchor)
		So(authority, ShouldEqual, 0.5)
		So(entered, ShouldBeLessThan, 0.10)
		So(entered, ShouldBeGreaterThan, 0)

		waited, _ := rehearsal.grade(episode, Action{Kind: "wait"}, selected, anchor)
		So(waited, ShouldEqual, -0.10)
	})
}
