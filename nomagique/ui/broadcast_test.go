package ui

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestBroadcast(t *testing.T) {
	Convey("Given a ui.Broadcast offramp", t, func() {
		var received any
		server := types.Value[any, any](func(in any) any {
			received = in
			return in
		})
		broadcast := NewBroadcast(server)

		Convey("When input arrives", func() {
			out := broadcast("test-data")
			So(out, ShouldEqual, "test-data")
			So(received, ShouldEqual, "test-data")
		})
	})
}
