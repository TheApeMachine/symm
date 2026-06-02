package user

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/public"
)

func TestDecodeExecutions(t *testing.T) {
	Convey("Given an empty executions snapshot", t, func() {
		message := &public.SocketMessage{
			Channel: public.ExecutionsChannel,
			Type:    "snapshot",
			Data:    []byte(`[]`),
		}

		Convey("It should decode zero rows", func() {
			rows, err := DecodeExecutions(message)

			So(err, ShouldBeNil)
			So(len(rows), ShouldEqual, 0)
		})
	})
}
