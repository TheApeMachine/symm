package ui

import (
	"testing"

	disruptor "github.com/smarty/go-disruptor"
	. "github.com/smartystreets/goconvey/convey"
)

func TestFrontendLinkConsumeSkip(t *testing.T) {
	Convey("Given a frontend link with pending skip marks", t, func() {
		link := &frontendLink{}

		link.skipOldest.Store(2)

		Convey("It should consume one skip mark per call", func() {
			So(link.consumeSkip(), ShouldBeTrue)
			So(link.consumeSkip(), ShouldBeTrue)
			So(link.consumeSkip(), ShouldBeFalse)
			So(link.skipOldest.Load(), ShouldEqual, 0)
		})
	})
}

func TestFrontendLinkPublishSignalsSkipWhenSaturated(t *testing.T) {
	Convey("Given a full frontend outbound disruptor ring", t, func() {
		const bufferSize = 4

		link := &frontendLink{
			ring: make([]outboundEvent, bufferSize),
		}
		handler := &frontendOutboundHandler{link: link}

		instance, err := disruptor.New(
			disruptor.Options.BufferCapacity(bufferSize),
			disruptor.Options.WriterCount(1),
			disruptor.Options.NewHandlerGroup(handler),
		)

		So(err, ShouldBeNil)

		link.outbound = instance

		for index := range bufferSize {
			sequence := instance.TryReserve(1)

			So(sequence, ShouldBeGreaterThanOrEqualTo, 0)
			link.ring[sequence&(bufferSize-1)] = outboundEvent{value: index}
			instance.Commit(sequence, sequence)
		}

		Convey("It should signal skip instead of returning saturation", func() {
			So(instance.TryReserve(1), ShouldEqual, disruptor.ErrCapacityUnavailable)
			So(link.publish("latest"), ShouldBeNil)
			So(link.skipOldest.Load(), ShouldBeGreaterThan, 0)
		})
	})
}
