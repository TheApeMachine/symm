package strategy

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
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
		Convey("Missing fees and regressed clocks fail explicitly", func() {
			So(evaluation.Grade(tape, broker.NewPrice(nil, nil)), ShouldNotBeNil)
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
