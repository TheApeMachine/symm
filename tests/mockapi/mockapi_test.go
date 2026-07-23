package mockapi

import (
	"encoding/json"
	"errors"
	"io"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

/*
TestMockConnWriteHonorsVenueBoundary proves failed and successful subscription
requests cannot leak unrequested market frames onto Actor roots.
*/
func TestMockConnWriteHonorsVenueBoundary(t *testing.T) {
	Convey("Given a two-symbol venue response", t, func() {
		conn := NewConn(t.Context(), "SIM1/USD", "SIM2/USD")
		sub := conn.Subscribe("ticker")
		payload := []byte(`{"channel":"ticker","type":"snapshot","data":[` +
			`{"symbol":"SIM1/USD"},{"symbol":"SIM2/USD"}]}`)
		conn.Respond("ticker", payload)
		request := json.RawMessage(
			`{"method":"subscribe","params":{"channel":"ticker","symbol":["SIM1/USD"]}}`,
		)

		Convey("A failed write emits no data", func() {
			conn.FailWrites(errors.New("write failed"))
			So(conn.Write(request), ShouldNotBeNil)
			So(len(sub.Channel), ShouldEqual, 0)
		})

		Convey("A successful write records and filters the subscription", func() {
			So(conn.Write(request), ShouldBeNil)
			So(conn.Subscriptions("ticker"), ShouldResemble, []string{"SIM1/USD"})
			frame := (<-sub.Channel).(*kraken.Ticker)
			So(frame.Data, ShouldHaveLength, 1)
			So(frame.Data[0].Symbol, ShouldEqual, "SIM1/USD")
			So(conn.Publish("ticker", payload), ShouldBeNil)
			So(conn.Drain(), ShouldBeNil)
			frame = (<-sub.Channel).(*kraken.Ticker)
			So(frame.Data, ShouldHaveLength, 1)
			So(frame.Data[0].Symbol, ShouldEqual, "SIM1/USD")
			So(conn.Publish("ticker", []byte(
				`{"channel":"ticker","type":"update","data":[{"symbol":"SIM2/USD"}]}`,
			)), ShouldBeNil)
			So(conn.Drain(), ShouldBeNil)
			So(len(sub.Channel), ShouldEqual, 0)
			So(conn.Write(json.RawMessage(
				`{"method":"unsubscribe","params":{"channel":"ticker","symbol":["SIM1/USD"]}}`,
			)), ShouldBeNil)
			So(conn.Publish("ticker", payload), ShouldBeNil)
			So(conn.Drain(), ShouldBeNil)
			So(len(sub.Channel), ShouldEqual, 0)
		})
	})

	Convey("Given a symbol-less private channel", t, func() {
		conn := NewConn(t.Context())
		sub := conn.Subscribe("balances")
		payload := []byte(`{"channel":"balances","type":"update","data":[]}`)

		Convey("Updates should begin only after its subscription is accepted", func() {
			So(conn.Publish("balances", payload), ShouldBeNil)
			So(conn.Drain(), ShouldBeNil)
			So(len(sub.Channel), ShouldEqual, 0)
			So(conn.Write(json.RawMessage(
				`{"method":"subscribe","params":{"channel":"balances"}}`,
			)), ShouldBeNil)
			So(conn.Subscribed("balances"), ShouldBeTrue)
			So(conn.Publish("balances", payload), ShouldBeNil)
			So(conn.Drain(), ShouldBeNil)
			So((<-sub.Channel).([]byte), ShouldResemble, payload)
		})
	})

	Convey("Given malformed configured venue responses", t, func() {
		for _, payload := range [][]byte{
			[]byte(`{"channel":"ticker"`),
			[]byte(`{"channel":"ticker","type":"snapshot"}`),
		} {
			conn := NewConn(t.Context(), "SIM1/USD")
			sub := conn.Subscribe("ticker")
			conn.Respond("ticker", payload)

			So(conn.Write(json.RawMessage(
				`{"method":"subscribe","params":{"channel":"ticker",`+
					`"symbol":["SIM1/USD"]}}`,
			)), ShouldNotBeNil)
			So(len(sub.Channel), ShouldEqual, 0)
		}
	})
}

/*
TestMockConnWriteRejectsUnknownOperations proves unsupported venue operations
cannot silently succeed.
*/
func TestMockConnWriteRejectsUnknownOperations(t *testing.T) {
	Convey("Given unsupported websocket operations", t, func() {
		conn := NewConn(t.Context(), "SIM1/USD")

		So(conn.Write(json.RawMessage(`{"method":"unknown"}`)), ShouldNotBeNil)
		So(conn.Write(json.RawMessage(
			`{"method":"subscribe","params":{"channel":"unknown","symbol":["SIM1/USD"]}}`,
		)), ShouldNotBeNil)
	})
}

/*
TestMockConnDrainAndClose proves queued delivery is explicit and shutdown
rejects all subsequent transport activity.
*/
func TestMockConnDrainAndClose(t *testing.T) {
	Convey("Given a queued frame", t, func() {
		conn := NewConn(t.Context())
		sub := conn.Subscribe("ticker")
		So(conn.Queue("ticker", []byte(
			`{"channel":"ticker","type":"update","data":[{"symbol":"SIM1/USD"}]}`,
		)), ShouldBeNil)
		So(len(sub.Channel), ShouldEqual, 0)
		So(conn.Drain(), ShouldBeNil)
		So((<-sub.Channel), ShouldNotBeNil)

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
func TestMockConnPostRejectsMissingRoute(t *testing.T) {
	Convey("Given an unconfigured REST path", t, func() {
		conn := NewConn(t.Context())
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
		conn := NewConn(t.Context())
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

/*
TestMockConnBookStampsIncrement proves books leave the mock venue only after
instrument increments are applied, matching Live ingress.
*/
func TestMockConnBookStampsIncrement(t *testing.T) {
	Convey("Given instrument and book fixtures on one mock venue", t, func() {
		conn := NewConn(t.Context(), "SIM1/USD")
		sub := conn.Subscribe("book")
		conn.Respond("instrument", []byte(
			`{"channel":"instrument","type":"snapshot","data":{"pairs":[{`+
				`"symbol":"SIM1/USD","quote":"USD","status":"online",`+
				`"price_increment":"0.01","tick_size":"0.01"}]}}`,
		))
		conn.Respond("book", []byte(
			`{"channel":"book","type":"snapshot","data":[{`+
				`"symbol":"SIM1/USD","bids":[{"price":"1.00","qty":1}],`+
				`"asks":[{"price":"1.01","qty":1}]}]}`,
		))

		Convey("When instrument then book are subscribed", func() {
			So(conn.Write(json.RawMessage(
				`{"method":"subscribe","params":{"channel":"instrument"}}`,
			)), ShouldBeNil)
			So(conn.Write(json.RawMessage(
				`{"method":"subscribe","params":{"channel":"book","symbol":["SIM1/USD"]}}`,
			)), ShouldBeNil)

			Convey("Then the book arrives stamped with the venue increment", func() {
				frame := (<-sub.Channel).(*kraken.Book)
				So(frame.Data[0].PriceIncrement, ShouldNotBeNil)
				So(frame.Data[0].PriceIncrement.Float64(), ShouldEqual, 0.01)
			})
		})
	})
}

