package runtime

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
)

/*
countingNode counts Step invocations and produces a fresh measurement when
the consumer registers it.
*/
type countingNode struct {
	steps int
}

func (node *countingNode) Step(state *data.Measurement[float64]) *data.Measurement[float64] {
	node.steps++

	return state
}

func (node *countingNode) Register() *data.Measurement[float64] {
	return data.NewMeasurement[float64]("counting", nil)
}

func TestConsumerHandle(t *testing.T) {
	Convey("Given a node bound to a register", t, func() {
		register := store.NewRegister[*data.Measurement[float64]]()
		node := &countingNode{}
		consumer := NewConsumer(node, register)

		Convey("the consumer identified the node's register slot", func() {
			So(consumer.Identity(), ShouldEqual, 0)
		})

		Convey("Handle reads the registered state, steps, and stores the return", func() {
			consumer.Handle(0, 0)

			So(node.steps, ShouldEqual, 1)

			measurement := data.Read[*data.Measurement[float64]](register.Next(
				data.NewValue(*store.NewQuery(consumer, data.ActionRead)),
			))

			So(measurement, ShouldNotBeNil)
			So(measurement.Source, ShouldEqual, "counting")
		})

		Convey("each sequence steps the node once more", func() {
			consumer.Handle(2, 5)

			So(node.steps, ShouldEqual, 4)
		})
	})
}
