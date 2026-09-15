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
		consumer := NewConsumer(t.Context(), node, register)
		consumer.Transition(READY)

		Convey("Paused consumers drop a range and resume without replaying it", func() {
			consumer.Transition(WAITING)
			consumer.Handle(0, 3)
			So(node.steps, ShouldEqual, 0)
			consumer.Transition(READY)
			consumer.Handle(4, 4)
			So(node.steps, ShouldEqual, 1)
		})

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

	Convey("Given a peer-aware node registered alongside a source node", t, func() {
		register := store.NewRegister[*data.Measurement[float64]]()
		sourceNode := &countingNode{}
		sourceConsumer := NewConsumer(t.Context(), sourceNode, register)
		sourceConsumer.Transition(READY)

		peerNode := &peerAwareNode{}
		peerConsumer := NewConsumer(t.Context(), peerNode, register)
		peerConsumer.Transition(READY)

		So(sourceConsumer.Identity(), ShouldEqual, 0)
		So(peerConsumer.Identity(), ShouldEqual, 1)

		Convey("Handle packages requested peer measurements into Peers", func() {
			sourceConsumer.Handle(0, 0)
			peerConsumer.Handle(0, 0)

			So(len(peerNode.lastPeers), ShouldEqual, 1)
			So(peerNode.lastPeers[0].Source, ShouldEqual, "counting")
		})
	})
}

type peerAwareNode struct {
	lastPeers []*data.Measurement[float64]
}

func (node *peerAwareNode) Step(state *data.Measurement[float64]) *data.Measurement[float64] {
	node.lastPeers = append([]*data.Measurement[float64](nil), state.Peers...)
	return state
}

func (node *peerAwareNode) Register() *data.Measurement[float64] {
	measurement := data.NewMeasurement[float64]("solver", nil)
	// The source consumer owns register slot zero in this fixture.
	measurement.Metadata["peer-interest"] = "0"

	return measurement
}

func BenchmarkConsumerHandle(b *testing.B) {
	register := store.NewRegister[*data.Measurement[float64]]()
	consumer := NewConsumer(b.Context(), &countingNode{}, register)
	consumer.Transition(READY)
	b.ResetTimer()
	for sequence := 0; sequence < b.N; sequence++ {
		consumer.Handle(int64(sequence), int64(sequence))
	}
	b.StopTimer()
	if err := consumer.Close(); err != nil {
		b.Fatal(err)
	}
}
