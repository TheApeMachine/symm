package strategy

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/cognition"
	"testing"
	"time"
)

func TestEvaluationGrade(t *testing.T) {
	Convey("Replay feedback measures secured profit and unproductive time", t, func() {
		tape := rehearsalObservations()
		price := practicePrice()
		evaluation := Evaluation{Initial: decimal.NewFromInt64(200), Quantity: decimal.NewFromInt64(1), Secured: decimal.NewFromInt64(10)}
		So(evaluation.Grade(tape, price), ShouldBeNil)
		So(evaluation.Value, ShouldAlmostEqual, .05)
		So(evaluation.Potential.Sign(), ShouldEqual, 1)
		So(evaluation.CaptureFraction, ShouldBeGreaterThan, 0)
		earned := evaluation.Value

		Convey("The measured grade changes inference through the packed cognitive weight", func() {
			engine := cognition.NewEngine(cognition.DefaultConfig())
			precursor := []byte("formation/ignition")
			engine.Observe(precursor, []byte("enter"), earned)
			evaluation.Secured = decimal.NewFromInt64(0)
			evaluation.Idle = tape[len(tape)-1].ReceivedAt.Sub(tape[0].ReceivedAt)
			So(evaluation.Grade(tape, price), ShouldBeNil)
			engine.Observe(precursor, []byte("wait"), evaluation.Value)
			So(engine.Evaluate(precursor).WinnerClass, ShouldEqual, "enter")

			// Repeated capital loss reverses the earlier entry association.
			evaluation.Secured = decimal.NewFromInt64(-10)
			So(evaluation.Grade(tape, price), ShouldBeNil)
			for range 32 {
				engine.Observe(precursor, []byte("enter"), evaluation.Value)
			}
			So(engine.Evaluate(precursor).WinnerClass, ShouldEqual, "wait")
		})

		Convey("Equal profit earned with less idle time scores higher", func() {
			evaluation.Idle = time.Second
			So(evaluation.Grade(tape, price), ShouldBeNil)
			So(evaluation.Value, ShouldBeLessThan, earned)
			So(evaluation.Value, ShouldBeGreaterThan, 0)
		})
		Convey("Waiting forever misses a payable opportunity", func() {
			evaluation.Secured = decimal.NewFromInt64(0)
			evaluation.Idle = tape[len(tape)-1].ReceivedAt.Sub(tape[0].ReceivedAt)
			So(evaluation.Grade(tape, price), ShouldBeNil)
			So(evaluation.Value, ShouldBeLessThan, 0)
		})
		Convey("Time does not excuse capital loss", func() {
			evaluation.Secured = decimal.NewFromInt64(-10)
			evaluation.Idle = time.Second
			So(evaluation.Grade(tape, price), ShouldBeNil)
			So(evaluation.Value, ShouldAlmostEqual, -.05)
		})
		Convey("An unexecutable tape does not reward or punish staying flat", func() {
			for index := range tape {
				tape[index].HasBid = false
			}
			evaluation.Secured = decimal.NewFromInt64(0)
			evaluation.Idle = time.Second
			So(evaluation.Grade(tape, price), ShouldBeNil)
			So(evaluation.Potential, ShouldBeNil)
			So(evaluation.Value, ShouldEqual, 0)
		})
		Convey("A batched frame's observations share one receive instant", func() {
			// A trade or touch message carrying several entries decodes into
			// several observations of the same frame, separated by ordinal
			// alone. That is an ordinary tape, not a disordered one.
			batched := append([]hindsight.Observation(nil), tape...)
			batched[1].Capture = batched[0].Capture
			batched[1].Ordinal, batched[1].ReceivedAt = 1, batched[0].ReceivedAt
			So(evaluation.Grade(batched, price), ShouldBeNil)
			So(evaluation.Value, ShouldAlmostEqual, earned)
		})
		Convey("Missing fees, mixed symbols and regressed clocks fail explicitly", func() {
			So(evaluation.Grade(tape, broker.NewPrice(nil, nil)), ShouldNotBeNil)
			mixed := append([]hindsight.Observation(nil), tape...)
			mixed[1].Symbol = "ETH/USD"
			So(evaluation.Grade(mixed, price), ShouldNotBeNil)

			reordered := append([]hindsight.Observation(nil), tape...)
			reordered[0], reordered[1] = reordered[1], reordered[0]
			So(evaluation.Grade(reordered, price), ShouldNotBeNil)
			tape[1].ReceivedAt = tape[0].ReceivedAt.Add(-time.Second)
			So(evaluation.Grade(tape, price), ShouldNotBeNil)
		})
	})
}

func BenchmarkEvaluationGrade(b *testing.B) {
	tape := rehearsalObservations()
	price := practicePrice()
	evaluation := Evaluation{Initial: decimal.NewFromInt64(200), Quantity: decimal.NewFromInt64(1), Secured: decimal.NewFromInt64(10)}
	b.ReportAllocs()
	for b.Loop() {
		if err := evaluation.Grade(tape, price); err != nil {
			b.Fatal(err)
		}
	}
}
