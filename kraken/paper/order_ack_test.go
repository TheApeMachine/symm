package paper

import (
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRejectedExecution(t *testing.T) {
	Convey("Given a rejected client order id", t, func() {
		message := rejectedExecution("cl-123", "insufficient funds")

		Convey("It should emit an executions update frame", func() {
			channel, _ := message["channel"].(string)
			data, _ := message["data"].(json.RawMessage)

			So(channel, ShouldEqual, "executions")
			So(string(data), ShouldContainSubstring, `"cl_ord_id":"cl-123"`)
			So(string(data), ShouldContainSubstring, `"exec_type":"rejected"`)
		})
	})
}

func TestClOrdIDFromOrder(t *testing.T) {
	Convey("Given an add_order frame", t, func() {
		clOrdID := clOrdIDFromOrder(map[string]any{
			"params": map[string]any{
				"cl_ord_id": "paper-abc",
				"symbol":    "BTC/EUR",
			},
		})

		Convey("It should extract the client order id", func() {
			So(clOrdID, ShouldEqual, "paper-abc")
		})
	})
}
