package data

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestKeyEpoch(t *testing.T) {
	Convey("KeyEpoch parses the epoch under a Prefix", t, func() {
		epoch, err := KeyEpoch(TickPrefix(), TickPrefix(1792184828503905000))
		So(err, ShouldBeNil)
		So(epoch, ShouldEqual, uint64(1792184828503905000))

		_, err = KeyEpoch(TickPrefix(), "run/epoch/12/")
		So(err, ShouldNotBeNil)

		_, err = KeyEpoch(TickPrefix(), TickPrefix()+"12/")
		So(err, ShouldNotBeNil)
	})
}
