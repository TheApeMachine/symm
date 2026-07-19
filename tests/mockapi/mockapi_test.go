package mockapi

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
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

/*
TestMockConnWriteRecordsRequests proves the producer boundary observes outbound
subscriptions and can expose transport failures to production callers.
*/
func TestMockConnWriteRecordsRequests(t *testing.T) {
	Convey("Given a mock connection configured to fail writes", t, func() {
		conn := &MockConn{}
		writeErr := errors.New("write failed")
		conn.FailWrites(writeErr)

		Convey("When production sends a ticker subscription", func() {
			err := conn.Write(kraken.NewTickerSubscription([]string{"BTC/USD"}))

			Convey("Then the request is recorded and the failure is returned", func() {
				So(err, ShouldEqual, writeErr)
				So(conn.Writes(), ShouldHaveLength, 1)
				So(string(conn.Writes()[0]), ShouldContainSubstring, `"channel":"ticker"`)
			})
		})
	})
}
