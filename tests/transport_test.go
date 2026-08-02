package tests

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	testtypes "github.com/theapemachine/symm/tests/types"
)

func TestMockTransportAssets(t *testing.T) {
	Convey("Given a mockTransport configured with simulated symbols", t, func() {
		symbols := []*testtypes.Symbol{
			testtypes.NewSymbol("BTC/USD", 50000, 1),
			testtypes.NewSymbol("ETH/USD", 3000, 2),
		}

		conn := NewConn(context.Background())
		conn.Configure(symbols)
		defer conn.Close()

		Convey("When the REST Assets endpoint is called", func() {
			request, err := http.NewRequest(
				"GET", "https://api.kraken.com/0/public/Assets", nil,
			)
			So(err, ShouldBeNil)

			response, err := conn.ws.REST.Executor(request)
			So(err, ShouldBeNil)
			So(response.StatusCode, ShouldEqual, 200)

			body, err := io.ReadAll(response.Body)
			So(err, ShouldBeNil)

			var result map[string]any
			So(json.Unmarshal(body, &result), ShouldBeNil)

			assets, _ := result["result"].(map[string]any)

			Convey("It should contain entries for BTC, USD, and ETH", func() {
				So(assets, ShouldContainKey, "BTC")
				So(assets, ShouldContainKey, "USD")
				So(assets, ShouldContainKey, "ETH")
			})
		})
	})
}

func TestMockTransportAssetPairs(t *testing.T) {
	Convey("Given a mockTransport configured with simulated symbols", t, func() {
		symbols := []*testtypes.Symbol{
			testtypes.NewSymbol("BTC/USD", 50000, 1),
		}

		conn := NewConn(context.Background())
		conn.Configure(symbols)
		defer conn.Close()

		Convey("When the REST AssetPairs endpoint is called", func() {
			request, err := http.NewRequest(
				"GET", "https://api.kraken.com/0/public/AssetPairs", nil,
			)
			So(err, ShouldBeNil)

			response, err := conn.ws.REST.Executor(request)
			So(err, ShouldBeNil)

			body, err := io.ReadAll(response.Body)
			So(err, ShouldBeNil)

			var result map[string]any
			So(json.Unmarshal(body, &result), ShouldBeNil)

			pairs, _ := result["result"].(map[string]any)

			Convey("It should contain the BTCUSD pair with wsname", func() {
				So(pairs, ShouldContainKey, "BTCUSD")

				pair, _ := pairs["BTCUSD"].(map[string]any)
				So(pair["wsname"], ShouldEqual, "BTC/USD")
				So(pair["base"], ShouldEqual, "BTC")
				So(pair["quote"], ShouldEqual, "USD")
			})
		})
	})
}

func TestMockTransportBalance(t *testing.T) {
	Convey("Given a mockTransport configured with symbols", t, func() {
		conn := NewConn(context.Background())
		conn.Configure([]*testtypes.Symbol{testtypes.NewSymbol("BTC/USD", 50000, 1)})
		defer conn.Close()

		Convey("When the REST Balance endpoint is called", func() {
			request, err := http.NewRequest(
				"POST", "https://api.kraken.com/0/private/Balance", nil,
			)
			So(err, ShouldBeNil)

			response, err := conn.ws.REST.Executor(request)
			So(err, ShouldBeNil)

			body, err := io.ReadAll(response.Body)
			So(err, ShouldBeNil)

			var result map[string]any
			So(json.Unmarshal(body, &result), ShouldBeNil)

			balanceResult, _ := result["result"].(map[string]any)

			Convey("It should contain a USD balance from the fixture", func() {
				So(balanceResult, ShouldContainKey, "USD")
			})
		})
	})
}

func TestMockTransportTradeVolume(t *testing.T) {
	Convey("Given a mockTransport configured with symbols", t, func() {
		symbols := []*testtypes.Symbol{
			testtypes.NewSymbol("BTC/USD", 50000, 1),
		}

		conn := NewConn(context.Background())
		conn.Configure(symbols)
		defer conn.Close()

		Convey("When the REST TradeVolume endpoint is called", func() {
			request, err := http.NewRequest(
				"POST", "https://api.kraken.com/0/private/TradeVolume", nil,
			)
			So(err, ShouldBeNil)

			response, err := conn.ws.REST.Executor(request)
			So(err, ShouldBeNil)

			body, err := io.ReadAll(response.Body)
			So(err, ShouldBeNil)

			var result map[string]any
			So(json.Unmarshal(body, &result), ShouldBeNil)

			volumeResult, _ := result["result"].(map[string]any)

			Convey("It should contain fees from the tradevolume fixture", func() {
				So(volumeResult, ShouldContainKey, "fees")

				fees, _ := volumeResult["fees"].(map[string]any)
				So(fees, ShouldContainKey, "BTCUSD")
			})
		})
	})
}

func TestMockTransportAddOrder(t *testing.T) {
	Convey("Given a mockTransport configured with symbols", t, func() {
		conn := NewConn(context.Background())
		conn.Configure([]*testtypes.Symbol{testtypes.NewSymbol("BTC/USD", 50000, 1)})
		defer conn.Close()

		Convey("When the REST AddOrder endpoint is called", func() {
			request, err := http.NewRequest(
				"POST", "https://api.kraken.com/0/private/AddOrder", nil,
			)
			So(err, ShouldBeNil)

			response, err := conn.ws.REST.Executor(request)
			So(err, ShouldBeNil)

			body, err := io.ReadAll(response.Body)
			So(err, ShouldBeNil)

			var result map[string]any
			So(json.Unmarshal(body, &result), ShouldBeNil)

			orderResult, _ := result["result"].(map[string]any)

			Convey("It should contain a txid from the orderack fixture", func() {
				So(orderResult, ShouldContainKey, "txid")

				txids, _ := orderResult["txid"].([]any)
				So(len(txids), ShouldBeGreaterThan, 0)
				So(txids[0].(string), ShouldStartWith, "SIM-ORD-")
			})
		})
	})
}

func BenchmarkMockTransportRoundTrip(b *testing.B) {
	symbols := []*testtypes.Symbol{
		testtypes.NewSymbol("BTC/USD", 50000, 1),
		testtypes.NewSymbol("ETH/USD", 3000, 2),
	}

	conn := NewConn(context.Background())
	conn.Configure(symbols)
	defer conn.Close()

	request, _ := http.NewRequest(
		"GET", "https://api.kraken.com/0/public/Assets", nil,
	)

	for b.Loop() {
		response, _ := conn.ws.REST.Executor(request)
		response.Body.Close()
	}
}
