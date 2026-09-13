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

	Convey("Given a peer-aware node registered alongside a source node", t, func() {
		register := store.NewRegister[*data.Measurement[float64]]()
		sourceNode := &countingNode{}
		sourceConsumer := NewConsumer(sourceNode, register)

		peerNode := &peerAwareNode{}
		peerConsumer := NewConsumer(peerNode, register)

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

func (node *peerAwareNode) Register() (*data.Measurement[float64], []string) {
	return data.NewMeasurement[float64]("solver", nil), []string{"counting"}
}
