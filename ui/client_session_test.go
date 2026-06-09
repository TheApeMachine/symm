package ui

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestClientSessionPublishDropsOldestWhenSaturated(t *testing.T) {
	Convey("Given a client session with a full outbound ring", t, func() {
		session := &clientSession{
			outbound: make(chan outboundEvent, 2),
		}

		session.outbound <- outboundEvent{value: "first"}
		session.outbound <- outboundEvent{value: "second"}

		Convey("It should accept a new frame by dropping the oldest pending frame", func() {
			So(session.publish("third"), ShouldBeNil)
			So(len(session.outbound), ShouldEqual, 2)

			first := (<-session.outbound).value
			second := (<-session.outbound).value

			So(first, ShouldEqual, "second")
			So(second, ShouldEqual, "third")
		})
	})
}
