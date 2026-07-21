package mockapi

import (
	"encoding/json"
	"errors"
	"io"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/*
TestMockConnUnsubscribeDropsExactHandler keeps closed-position teardown from
clearing unrelated ticker consumers on the shared public channel.
*/
func TestMockConnUnsubscribeDropsExactHandler(t *testing.T) {
	Convey("Given two handlers on the same channel", t, func() {
		conn := NewConn()
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
TestMockConnWriteHonorsVenueBoundary proves failed and successful subscription
requests cannot leak unrequested market frames.
*/
func TestMockConnWriteHonorsVenueBoundary(t *testing.T) {
	Convey("Given a two-symbol venue response", t, func() {
		conn := NewConn("SIM1/USD", "SIM2/USD")
		payload := []byte(`{"channel":"ticker","type":"snapshot","data":[` +
			`{"symbol":"SIM1/USD"},{"symbol":"SIM2/USD"}]}`)
		conn.Respond("ticker", payload)
		received := [][]byte{}
		conn.On("ticker", func(frame []byte) { received = append(received, frame) })
		request := json.RawMessage(
			`{"method":"subscribe","params":{"channel":"ticker","symbol":["SIM1/USD"]}}`,
		)

		Convey("A failed write emits no data", func() {
			conn.FailWrites(errors.New("write failed"))
			So(conn.Write(request), ShouldNotBeNil)
			So(received, ShouldBeEmpty)
		})

		Convey("A successful write records and filters the subscription", func() {
			So(conn.Write(request), ShouldBeNil)
			So(conn.Subscriptions("ticker"), ShouldResemble, []string{"SIM1/USD"})
			So(received, ShouldHaveLength, 1)
			So(string(received[0]), ShouldContainSubstring, `"symbol":"SIM1/USD"`)
			So(string(received[0]), ShouldNotContainSubstring, `"symbol":"SIM2/USD"`)
		})
	})
}

/*
TestMockConnDrainAndClose proves scheduled delivery is explicit and shutdown
rejects all subsequent transport activity.
*/
func TestMockConnDrainAndClose(t *testing.T) {
	Convey("Given a queued frame", t, func() {
		conn := NewConn()
		hits := 0
		conn.On("ticker", func([]byte) { hits++ })
		So(conn.Queue("ticker", []byte(`{}`)), ShouldBeNil)
		So(hits, ShouldEqual, 0)
		So(conn.Drain(), ShouldBeNil)
		So(hits, ShouldEqual, 1)

		Convey("Closing rejects writes, REST, queueing, and draining", func() {
			conn.Close()
			request := json.RawMessage(`{"method":"subscribe","params":{"channel":"ticker"}}`)
			_, postErr := conn.Post("/missing", request)

			So(conn.Write(request), ShouldEqual, io.ErrClosedPipe)
			So(postErr, ShouldEqual, io.ErrClosedPipe)
			So(conn.Queue("ticker", []byte(`{}`)), ShouldEqual, io.ErrClosedPipe)
			So(conn.Drain(), ShouldEqual, io.ErrClosedPipe)
		})
	})
}

/*
TestMockConnPostRejectsMissingRoute prevents empty successful REST responses
from hiding an incomplete simulated venue.
*/
func TestMockConnPostRejectsMissingRoute(t *testing.T) {
	Convey("Given an unconfigured REST path", t, func() {
		conn := NewConn()
		_, err := conn.Post("/missing", json.RawMessage(`{}`))

		So(err, ShouldNotBeNil)
	})
}

/*
TestMockConnWriteRecordsRequests proves the producer boundary observes outbound
subscriptions and can expose transport failures to production callers.
*/
func TestMockConnWriteRecordsRequests(t *testing.T) {
	Convey("Given a mock connection configured to fail writes", t, func() {
		conn := NewConn()
		writeErr := errors.New("write failed")
		conn.FailWrites(writeErr)

		Convey("When production sends a ticker subscription", func() {
			err := conn.Write(json.RawMessage(
				`{"method":"subscribe","params":{"channel":"ticker","symbol":["BTC/USD"]}}`,
			))

			Convey("Then the request is recorded and the failure is returned", func() {
				So(err, ShouldEqual, writeErr)
				So(conn.Writes(), ShouldHaveLength, 1)
				So(string(conn.Writes()[0]), ShouldContainSubstring, `"channel":"ticker"`)
			})
		})
	})
}
