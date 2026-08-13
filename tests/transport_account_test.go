package tests

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	testtypes "github.com/theapemachine/symm/tests/types"
)

func TestMockTransportBalance(t *testing.T) {
	Convey("Given a transport configured with a USD-quoted symbol", t, func() {
		conn := NewConn(context.Background())
		defer conn.Close()
		conn.Configure([]*testtypes.Symbol{
			testtypes.NewSymbol("BTC/USD", 50_000, 1),
		})
		request, err := http.NewRequest(
			"POST", "https://api.kraken.com/0/private/Balance", nil,
		)
		So(err, ShouldBeNil)

		response, err := conn.ws.REST.Executor(request)
		So(err, ShouldBeNil)
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		So(err, ShouldBeNil)
		result := map[string]any{}
		So(json.Unmarshal(body, &result), ShouldBeNil)
		balances, _ := result["result"].(map[string]any)

		Convey("The fixture wallet should expose its USD inventory", func() {
			So(balances, ShouldContainKey, "USD")
		})
	})
}

func TestMockTransportTradeVolume(t *testing.T) {
	Convey("Given explicit maker and taker fees", t, func() {
		symbol := testtypes.NewSymbol("BTC/USD", 50_000, 1)
		symbol.TakerFeePercent = 0.31
		symbol.MakerFeePercent = 0.19
		conn := NewConn(context.Background())
		defer conn.Close()
		conn.Configure([]*testtypes.Symbol{symbol})
		request, err := http.NewRequest(
			"POST", "https://api.kraken.com/0/private/TradeVolume", nil,
		)
		So(err, ShouldBeNil)

		response, err := conn.ws.REST.Executor(request)
		So(err, ShouldBeNil)
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		So(err, ShouldBeNil)
		result := map[string]any{}
		So(json.Unmarshal(body, &result), ShouldBeNil)
		volume, _ := result["result"].(map[string]any)
		fees, _ := volume["fees"].(map[string]any)
		taker, _ := fees["BTCUSD"].(map[string]any)
		makers, _ := volume["fees_maker"].(map[string]any)
		maker, _ := makers["BTCUSD"].(map[string]any)

		Convey("REST should declare the same per-symbol fee schedule", func() {
			So(taker["fee"], ShouldEqual, "0.31")
			So(maker["fee"], ShouldEqual, "0.19")
		})
	})
}

func TestMockTransportTradeBalance(t *testing.T) {
	Convey("Given a fixture account with USD inventory", t, func() {
		conn := NewConn(context.Background())
		defer conn.Close()
		conn.Configure([]*testtypes.Symbol{
			testtypes.NewSymbol("BTC/USD", 50_000, 1),
		})
		request, err := http.NewRequest(
			"POST", "https://api.kraken.com/0/private/TradeBalance", nil,
		)
		So(err, ShouldBeNil)

		response, err := conn.ws.REST.Executor(request)
		So(err, ShouldBeNil)
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		So(err, ShouldBeNil)
		balance := kraken.NewTradeBalance(body)

		Convey("REST should return a complete internally consistent valuation", func() {
			So(balance.Equity, ShouldNotBeNil)
			So(balance.TradeBalance, ShouldNotBeNil)
			So(balance.UnrealizedPnL, ShouldNotBeNil)
			So(balance.Equity.Cmp(balance.TradeBalance.Add(balance.UnrealizedPnL)), ShouldEqual, 0)
		})
	})
}
