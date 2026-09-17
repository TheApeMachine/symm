package runtime

import (
	"iter"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data/sequence"
)

/*
consumerNode records iterator use without imposing a payload type on Consumer.
*/
type consumerNode struct {
	*core.PrimitiveError
	address string
	calls   int
	visits  int
}

func (node *consumerNode) Identity() string { return node.address }

func (node *consumerNode) Identify(address string) core.Identifiable[string] {
	node.address = address
	return node
}

func (node *consumerNode) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	node.calls++
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			node.visits++
			if !yield(arriving) {
				return
			}
		}
	}
}

func TestNewConsumer(t *testing.T) {
	Convey("A consumer binds an identifiable primitive without executing it", t, func() {
		node := &consumerNode{PrimitiveError: core.NewPrimitiveError(), address: "metric"}
		consumer := NewConsumer(node)
		So(consumer.node, ShouldEqual, node)
		So(consumer.Error(), ShouldBeNil)
		So(node.calls, ShouldEqual, 0)
	})
}

func TestConsumerNext(t *testing.T) {
	Convey("Consumer delegates arbitrary borrowed streams", t, func() {
		node := &consumerNode{PrimitiveError: core.NewPrimitiveError(), address: "metric"}
		consumer := NewConsumer(node)
		values := [3]string{"first", "second", "third"}
		input := sequence.NewValue(values[:]...)
		output := consumer.Next(input)
		So(node.calls, ShouldEqual, 1)
		So(node.visits, ShouldEqual, 0)

		Convey("Every address arrives unchanged and in order", func() {
			index := 0
			for pointer := range output {
				So(pointer, ShouldEqual, unsafe.Pointer(&values[index]))
				index++
			}
			So(index, ShouldEqual, len(values))
			So(node.visits, ShouldEqual, len(values))
		})

		Convey("Downstream termination stops upstream traversal", func() {
			for range output {
				break
			}
			So(node.visits, ShouldEqual, 1)
		})

		Convey("An empty stream does not invent an observation", func() {
			for range consumer.Next(sequence.NewValue[string]()) {
				t.Fatal("unexpected output")
			}
			So(node.visits, ShouldEqual, 0)
		})
	})
}

func BenchmarkConsumerNext(b *testing.B) {
	node := &consumerNode{PrimitiveError: core.NewPrimitiveError(), address: "metric"}
	consumer := NewConsumer(node)
	values := [3]float64{1, 2, 3}
	input := sequence.NewValue(values[:]...)
	b.ReportAllocs()
	for b.Loop() {
		for range consumer.Next(input) {
		}
	}
}
