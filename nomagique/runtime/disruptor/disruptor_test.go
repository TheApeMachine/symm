package disruptor_test

import (
	"iter"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime/disruptor"
)

/*
counter is a Handler that records how many sequences it was handed, which is
how a consumer proves it observed every event a producer published.
*/
type counter struct {
	seen atomic.Int64
}

func (handler *counter) Next(batch iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	for range batch {
		handler.seen.Add(1)
	}

	return batch
}

func TestDisruptor(t *testing.T) {
	Convey("Given a disruptor fanning one producer out to many consumers", t, func() {
		first := &counter{}
		second := &counter{}
		third := &counter{}

		ring, err := disruptor.New(
			disruptor.Options.BufferCapacity(1024),
			disruptor.Options.NewHandlerGroup(first, second, third),
		)
		So(err, ShouldBeNil)
		So(ring, ShouldNotBeNil)

		go ring.Listen()

		const published = 4096

		for index := 0; index < published; index++ {
			sequence := ring.Reserve(1)
			ring.Commit(sequence, sequence)
		}

		deadline := time.After(5 * time.Second)

		for first.seen.Load() < published ||
			second.seen.Load() < published ||
			third.seen.Load() < published {

			select {
			case <-deadline:
				t.Fatalf(
					"consumers stalled: %d %d %d of %d",
					first.seen.Load(), second.seen.Load(), third.seen.Load(), published,
				)
			default:
				time.Sleep(time.Millisecond)
			}
		}

		Convey("Every consumer observes every published event", func() {
			So(first.seen.Load(), ShouldEqual, published)
			So(second.seen.Load(), ShouldEqual, published)
			So(third.seen.Load(), ShouldEqual, published)
		})

		So(ring.Close(), ShouldBeNil)
	})

	Convey("Given a disruptor with concurrent producers", t, func() {
		consumer := &counter{}

		ring, err := disruptor.New(
			disruptor.Options.BufferCapacity(1024),
			disruptor.Options.WriterCount(4),
			disruptor.Options.NewHandlerGroup(consumer),
		)
		So(err, ShouldBeNil)

		go ring.Listen()

		const perWriter = 1024
		const writers = 4

		done := make(chan struct{}, writers)

		for writer := 0; writer < writers; writer++ {
			go func() {
				for index := 0; index < perWriter; index++ {
					sequence := ring.Reserve(1)
					ring.Commit(sequence, sequence)
				}

				done <- struct{}{}
			}()
		}

		for writer := 0; writer < writers; writer++ {
			<-done
		}

		deadline := time.After(5 * time.Second)

		for consumer.seen.Load() < perWriter*writers {
			select {
			case <-deadline:
				t.Fatalf("consumer stalled at %d of %d", consumer.seen.Load(), perWriter*writers)
			default:
				time.Sleep(time.Millisecond)
			}
		}

		Convey("The consumer observes every event from every producer", func() {
			So(consumer.seen.Load(), ShouldEqual, perWriter*writers)
		})

		So(ring.Close(), ShouldBeNil)
	})

	Convey("Given an invalid configuration", t, func() {
		Convey("It rejects a capacity that is not a power of two", func() {
			_, err := disruptor.New(
				disruptor.Options.BufferCapacity(1000),
				disruptor.Options.NewHandlerGroup(&counter{}),
			)
			So(err, ShouldNotBeNil)
		})

		Convey("It rejects a configuration with no handlers", func() {
			_, err := disruptor.New(disruptor.Options.BufferCapacity(1024))
			So(err, ShouldNotBeNil)
		})
	})
}
