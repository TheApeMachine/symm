package runtime

import (
	"errors"
	"sync/atomic"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

var consumerBenchmark int

type failingNode struct {
	failure error
	err     error
}

func (node *failingNode) Step(value int) int {
	node.err = node.failure

	return value
}

func (node *failingNode) Error() error { return node.err }

type failingBacklogNode struct {
	failingNode
}

func (node *failingBacklogNode) StepBacklog(value int, _ int64) int {
	return node.Step(value)
}

type passingNode struct{}

func (passingNode) Step(value int) int { return value }

func TestConsumerHandle(t *testing.T) {
	Convey("Given an ordinary node that latches a terminal error", t, func() {
		err := errors.New("node failed")
		consumer := NewConsumer(
			&failingNode{failure: err}, []int{1}, &atomic.Int64{},
		)

		Convey("Handle halts on the same event", func() {
			So(func() { consumer.Handle(0, 0) }, ShouldPanicWith, err)
		})
	})

	Convey("Given a backlog-aware node that latches a terminal error", t, func() {
		err := errors.New("node failed")
		node := &failingBacklogNode{
			failingNode: failingNode{failure: err},
		}
		consumer := NewConsumer(node, []int{1}, &atomic.Int64{})

		Convey("Handle halts on the same event", func() {
			So(func() { consumer.Handle(0, 0) }, ShouldPanicWith, err)
		})
	})
}

func BenchmarkConsumerHandle(b *testing.B) {
	buffer := make([]int, 1)
	head := &atomic.Int64{}
	consumer := NewConsumer(passingNode{}, buffer, head)
	b.ReportAllocs()

	for b.Loop() {
		consumer.Handle(0, 0)
		consumerBenchmark = buffer[0]
	}
}
