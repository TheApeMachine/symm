package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestNewDesk(t *testing.T) {
	Convey("Given a newly constructed Desk", t, func() {
		desk := NewDesk(nil, nil, nil, make(chan []byte, 1))

		Convey("Then it waits for balance and price before hydrating", func() {
			So(desk.Status(), ShouldEqual, types.INITIALIZING)
		})
	})
}
