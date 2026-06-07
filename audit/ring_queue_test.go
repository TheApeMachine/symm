package audit

import (
	"sync"
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestRingQueuePush(t *testing.T) {
	convey.Convey("Given an open ring queue", t, func() {
		queue := newRingQueue()

		convey.Convey("It should return frames in enqueue order", func() {
			convey.So(queue.Push(map[string]any{"seq": 1}), convey.ShouldBeNil)
			convey.So(queue.Push(map[string]any{"seq": 2}), convey.ShouldBeNil)

			first, ok := queue.Pop()
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(first["seq"], convey.ShouldEqual, 1)

			second, ok := queue.Pop()
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(second["seq"], convey.ShouldEqual, 2)
		})
	})
}

func TestRingQueueClose(t *testing.T) {
	convey.Convey("Given a closed ring queue", t, func() {
		queue := newRingQueue()
		queue.Close()

		convey.Convey("It should reject new frames and let the worker stop", func() {
			convey.So(queue.Push(map[string]any{}), convey.ShouldNotBeNil)

			frame, ok := queue.Pop()
			convey.So(frame, convey.ShouldBeNil)
			convey.So(ok, convey.ShouldBeFalse)
		})
	})
}

func TestRingQueueConcurrentPush(t *testing.T) {
	queue := newRingQueue()
	const publishers = 8
	const framesPerPublisher = 32

	var waitGroup sync.WaitGroup
	waitGroup.Add(publishers)

	var pushErrors sync.Map

	for publisher := range publishers {
		go func(start int) {
			defer waitGroup.Done()

			for offset := range framesPerPublisher {
				err := queue.Push(map[string]any{
					"publisher": start,
					"offset":    offset,
				})

				if err != nil {
					pushErrors.Store(start, err)
				}
			}
		}(publisher)
	}

	waitGroup.Wait()

	pushErrors.Range(func(_, value any) bool {
		t.Fatalf("push failed: %v", value)

		return false
	})

	received := 0

	for received < publishers*framesPerPublisher {
		frame, ok := queue.Pop()

		if !ok {
			t.Fatalf("pop ended early at %d", received)
		}

		if frame == nil {
			t.Fatal("nil frame")
		}

		received++
	}
}

func BenchmarkWriterQueuePushPop(b *testing.B) {
	queue := newWriterQueue()
	frame := map[string]any{"audit_event": "playbook_walk", "symbol": "BTC/EUR"}

	for b.Loop() {
		if err := queue.Push(frame); err != nil {
			b.Fatal(err)
		}

		if _, ok := queue.Pop(); !ok {
			b.Fatal("pop failed")
		}
	}
}
