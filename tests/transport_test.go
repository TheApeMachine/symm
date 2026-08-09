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
	Convey("Given a transport configured with two simulated symbols", t, func() {
		conn := NewConn(context.Background())
		defer conn.Close()
		conn.Configure([]*testtypes.Symbol{
			testtypes.NewSymbol("BTC/USD", 50_000, 1),
			testtypes.NewSymbol("ETH/USD", 3_000, 2),
		})
		request, err := http.NewRequest(
			"GET", "https://api.kraken.com/0/public/Assets", nil,
		)
		So(err, ShouldBeNil)

		response, err := conn.ws.REST.Executor(request)
		So(err, ShouldBeNil)
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		So(err, ShouldBeNil)
		result := map[string]any{}
		So(json.Unmarshal(body, &result), ShouldBeNil)
		assets, _ := result["result"].(map[string]any)

		Convey("Every base and quote asset should be declared", func() {
			So(assets, ShouldContainKey, "BTC")
			So(assets, ShouldContainKey, "ETH")
			So(assets, ShouldContainKey, "USD")
		})
	})
}

func TestMockTransportAssetPairs(t *testing.T) {
	Convey("Given a transport configured with one symbol", t, func() {
		conn := NewConn(context.Background())
		defer conn.Close()
		conn.Configure([]*testtypes.Symbol{
			testtypes.NewSymbol("BTC/USD", 50_000, 1),
		})
		request, err := http.NewRequest(
			"GET", "https://api.kraken.com/0/public/AssetPairs", nil,
		)
		So(err, ShouldBeNil)

		response, err := conn.ws.REST.Executor(request)
		So(err, ShouldBeNil)
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		So(err, ShouldBeNil)
		result := map[string]any{}
		So(json.Unmarshal(body, &result), ShouldBeNil)
		pairs, _ := result["result"].(map[string]any)
		pair, _ := pairs["BTCUSD"].(map[string]any)

		Convey("The venue pair identity should match the websocket symbol", func() {
			So(pair["wsname"], ShouldEqual, "BTC/USD")
			So(pair["base"], ShouldEqual, "BTC")
			So(pair["quote"], ShouldEqual, "USD")
		})
	})
}

func BenchmarkMockTransportRoundTrip(b *testing.B) {
	conn := NewConn(context.Background())
	conn.Configure([]*testtypes.Symbol{
		testtypes.NewSymbol("BTC/USD", 50_000, 1),
		testtypes.NewSymbol("ETH/USD", 3_000, 2),
	})
	defer conn.Close()
	request, err := http.NewRequest(
		"GET", "https://api.kraken.com/0/public/Assets", nil,
	)

	if err != nil {
		b.Fatal(err)
	}

	for b.Loop() {
		response, err := conn.ws.REST.Executor(request)

		if err != nil {
			b.Fatal(err)
		}

		response.Body.Close()
	}
}
