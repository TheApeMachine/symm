package market

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

const opportunityFloatTolerance = 1e-12

func TestNewOpportunityTape(t *testing.T) {
	Convey("Given a deterministic multi-leg opportunity tape", t, func() {
		start := time.Unix(1_700_000_000, 0)
		tape := NewOpportunityTape("TEST/USD", start, 4)

		So(tape.Symbol, ShouldEqual, "TEST/USD")
		So(tape.HorizonSteps, ShouldEqual, 3)
		So(len(tape.Steps), ShouldEqual, 15)

		Convey("every observation carries ordered event time and an executable bid", func() {
			previous := start

			for _, step := range tape.Steps {
				So(step.EventTime.After(previous), ShouldBeTrue)
				So(step.ExecutableBid, ShouldBeGreaterThan, 0.0)
				previous = step.EventTime
			}
		})

		Convey("each signed context aligns with its three-tick future return", func() {
			positive := 0
			negative := 0

			for index := 0; index+tape.HorizonSteps < len(tape.Steps); index++ {
				step := tape.Steps[index]
				future := tape.Steps[index+tape.HorizonSteps]
				logReturn := math.Log(future.ExecutableBid / step.ExecutableBid)

				So(
					logReturn*step.Context,
					ShouldAlmostEqual,
					opportunityLogReturnSize,
					opportunityFloatTolerance,
				)

				if step.Context > 0 {
					positive++

					continue
				}

				negative++
			}

			So(positive, ShouldBeGreaterThan, 0)
			So(negative, ShouldBeGreaterThan, 0)
		})

		Convey("equal ticker horizons span unequal elapsed durations", func() {
			first := tape.Steps[tape.HorizonSteps].EventTime.Sub(tape.Steps[0].EventTime)
			second := tape.Steps[tape.HorizonSteps+1].EventTime.Sub(tape.Steps[1].EventTime)

			So(first, ShouldNotEqual, second)
		})

		Convey("the same request produces the same replay", func() {
			again := NewOpportunityTape("TEST/USD", start, 4)

			So(again, ShouldResemble, tape)
		})
	})
}

func BenchmarkNewOpportunityTape(b *testing.B) {
	start := time.Unix(1_700_000_000, 0)

	for b.Loop() {
		NewOpportunityTape("TEST/USD", start, 64)
	}
}
