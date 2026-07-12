package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestNewDesk(t *testing.T) {
	Convey("Given a newly constructed Desk", t, func() {
		desk := NewDesk(nil, nil, nil, make(chan []byte, 1))

		Convey("Then it declares itself ready immediately, since positions live only in its own in-memory map and there is no persisted state to hydrate", func() {
			So(desk.Status(), ShouldEqual, types.READY)
		})
	})
}
