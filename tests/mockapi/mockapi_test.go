package mockapi

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/*
TestMockConnUnsubscribeDropsExactHandler keeps closed-position teardown from
clearing unrelated ticker consumers on the shared public channel.
*/
func TestMockConnUnsubscribeDropsExactHandler(t *testing.T) {
	Convey("Given two handlers on the same channel", t, func() {
		conn := &MockConn{}
		firstHits := 0
		secondHits := 0
		first := func([]byte) { firstHits++ }
		second := func([]byte) { secondHits++ }

		firstID := conn.On("ticker", first)
		conn.On("ticker", second)
		conn.Unsubscribe("ticker", firstID)
		conn.Emit("ticker", []byte(`{}`))

		Convey("Then only the remaining handler receives the frame", func() {
			So(firstHits, ShouldEqual, 0)
			So(secondHits, ShouldEqual, 1)
		})
	})
}
