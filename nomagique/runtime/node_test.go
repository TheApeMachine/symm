package runtime

import (
	"errors"
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
	steps  int
	onStep func(*data.Measurement[float64]) *data.Measurement[float64]
}

func (node *countingNode) Step(state *data.Measurement[float64]) *data.Measurement[float64] {
	node.steps++

	if node.onStep != nil {
		return node.onStep(state)
	}

	return state
}

func (node *countingNode) Register() *data.Measurement[float64] {
	return data.NewMeasurement[float64]("counting", nil)
}

func TestConsumerHandle(t *testing.T) {
	Convey("Given a node bound to a register", t, func() {
		register := store.NewRegister[*data.Measurement[float64]](8)
		node := &countingNode{}
		consumer := NewConsumer(node, register)

		Convey("Consumers process consecutive committed ranges without lifecycle setup", func() {
			consumer.Handle(0, 3)
			So(node.steps, ShouldEqual, 4)
			consumer.Handle(4, 4)
			So(node.steps, ShouldEqual, 5)
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

		Convey("Committed observations are stamped before calculation and publication", func() {
			var inputs []int64
			node.onStep = func(measurement *data.Measurement[float64]) *data.Measurement[float64] {
				inputs = append(inputs, measurement.SeqIdx)
				return data.NewMeasurement[float64]("fresh-output", nil)
			}
			consumer.Handle(0, 1)
			consumer.Handle(2, 2)
			So(inputs, ShouldResemble, []int64{1, 2, 3})
			for sequence := int64(0); sequence < 3; sequence++ {
				query := store.NewQuery[*data.Measurement[float64]](nil, data.ActionRead).SetSequence(sequence)
				So(data.Read[*data.Measurement[float64]](register.Next(data.NewValue(*query))).SeqIdx, ShouldEqual, sequence+1)
			}
		})

		Convey("A node with no new observation does not republish its old register value", func() {
			node.onStep = func(measurement *data.Measurement[float64]) *data.Measurement[float64] {
				if node.steps == 2 {
					return nil
				}

				return measurement
			}
			consumer.Handle(0, 2)
			query := store.NewQuery[*data.Measurement[float64]](nil, data.ActionRead).SetSequence(1)
			So(data.Read[*data.Measurement[float64]](register.Next(data.NewValue(*query))), ShouldBeNil)
		})

		Convey("Observation errors stay on their recorded boundary and do not poison the next input", func() {
			node.onStep = func(measurement *data.Measurement[float64]) *data.Measurement[float64] {
				So(measurement.Err, ShouldBeNil)

				if node.steps == 1 {
					measurement.Err = errors.New("rejected observation")
				}
				return measurement
			}
			consumer.Handle(0, 1)
			query := store.NewQuery[*data.Measurement[float64]](nil, data.ActionRead).SetSequence(0)
			So(data.Read[*data.Measurement[float64]](register.Next(data.NewValue(*query))).Err, ShouldNotBeNil)
			query.SetSequence(1)
			So(data.Read[*data.Measurement[float64]](register.Next(data.NewValue(*query))).Err, ShouldBeNil)
		})

		Convey("Each calculation starts without the previous structured result", func() {
			node.onStep = func(measurement *data.Measurement[float64]) *data.Measurement[float64] {
				So(measurement.Result, ShouldBeNil)
				measurement.Result = node.steps
				return measurement
			}
			consumer.Handle(0, 2)
			So(node.steps, ShouldEqual, 3)
		})
	})

	Convey("Given a peer-aware node registered alongside a source node", t, func() {
		register := store.NewRegister[*data.Measurement[float64]](8)
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

func (node *peerAwareNode) Register() *data.Measurement[float64] {
	measurement := data.NewMeasurement[float64]("solver", nil)
	measurement.Metadata["peer-interest"] = "counting"

	return measurement
}

func BenchmarkConsumerHandle(b *testing.B) {
	register := store.NewRegister[*data.Measurement[float64]](8)
	consumer := NewConsumer(&countingNode{}, register)
	b.ResetTimer()
	for sequence := 0; sequence < b.N; sequence++ {
		consumer.Handle(int64(sequence), int64(sequence))
	}
}
