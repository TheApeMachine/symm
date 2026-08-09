package tests

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	testtypes "github.com/theapemachine/symm/tests/types"
)

func TestMockTransportAddOrder(t *testing.T) {
	Convey("Given a valid order for a declared symbol", t, func() {
		conn := NewConn(context.Background())
		defer conn.Close()
		conn.Configure([]*testtypes.Symbol{
			testtypes.NewSymbol("BTC/USD", 50_000, 1),
		})
		request, err := http.NewRequest(
			"POST", "https://api.kraken.com/0/private/AddOrder",
			strings.NewReader(`{
				"cl_ord_id":"client-order-1","ordertype":"market",
				"type":"buy","volume":"0.25","pair":"BTC/USD"
			}`),
		)
		So(err, ShouldBeNil)

		response, err := conn.ws.REST.Executor(request)
		So(err, ShouldBeNil)
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		So(err, ShouldBeNil)
		result := map[string]any{}
		So(json.Unmarshal(body, &result), ShouldBeNil)
		order, _ := result["result"].(map[string]any)
		identities, _ := order["txid"].([]any)

		Convey("It should return a venue identity and queue the exact order", func() {
			So(identities, ShouldHaveLength, 1)
			So(identities[0].(string), ShouldStartWith, "SIM-ORD-")
			So(conn.transport.pending, ShouldHaveLength, 1)
			So(conn.transport.pending[0].Request.Pair, ShouldEqual, "BTC/USD")
		})
	})
}
