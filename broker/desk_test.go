package broker

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/config"
	krakenmodel "github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/mockapi"
	"github.com/theapemachine/symm/types"
)

/*
TestDeskRecoversOpenLotsAfterInstrumentReady proves recovery can recreate an
already-open wallet lot when balances arrive before instruments during boot.
The real broker boot order initializes Desk first, then Balance, then
Instrument; this test keeps that order and requires the recovered position to
exist only after instrument and ticker state become available, with a seeded
live mark and active stoploss instead of the zeroed ghost shell.
*/
func TestDeskRecoversOpenLotsAfterInstrumentReady(t *testing.T) {
	Convey("Given a pre-existing wallet lot before Balance and Instrument finish booting", t, func() {
		market := tests.NewMarket(t.Context(), 1)
		cfg := config.Fixture()
		cfg.Market.L3Enabled = false
		symbol := market.Symbols[0]
		ui := make(chan []byte, 1024)

		So(market.Paper.EnablePaper(mockapi.PaperOptions{
			Quote: func(symbol string) (float64, float64, float64, float64, bool) {
				return 101, 5, 102, 5, symbol == "SIM1/USD"
			},
			Now: market.Now,
			Balances: map[string]float64{
				"USD":  80595.4943,
				"SIM1": 2,
			},
			MakerFee: 0.0016,
			TakerFee: 0.0026,
		}), ShouldBeNil)
		So(market.Bootstrap(), ShouldBeNil)
		Reset(market.Close)

		api := websocket.NewAPI(t.Context(), market.Public, market.Private, market.Paper)
		price := NewPrice(api)
		instrument := NewInstrument(api, price, ui, cfg.Market)
		balance := NewBalance(api, ui, cfg.Market)
		desk := NewDesk(t.Context(), api, instrument, price, balance, cfg.Trading)

		So(api.Initialize(), ShouldBeNil)
		So(price.Initialize(), ShouldBeNil)
		So(desk.Initialize(market.Public.Root(), api.Account().Root()), ShouldBeNil)
		So(balance.Initialize(), ShouldBeNil)
		So(instrument.Initialize(), ShouldBeNil)

		Convey("Then the lot is recreated as a real Position after instrument and ticker state become available", func() {
			deadline := time.Now().Add(2 * time.Second)
			var position *Position
			var ok bool

			for time.Now().Before(deadline) {
				position, ok = desk.Position(symbol)

				if ok && position != nil && position.Holding != nil &&
					position.Holding.Status == types.PENDING &&
					position.Holding.Mark != nil && position.Holding.Mark.Sign() > 0 &&
					position.Holding.Stoploss != nil &&
					position.Holding.Stoploss.Mark != nil && position.Holding.Stoploss.Mark.Sign() > 0 &&
					position.Holding.Stoploss.Floor != nil && position.Holding.Stoploss.Floor.Sign() > 0 {
					break
				}

				time.Sleep(10 * time.Millisecond)
			}

			So(ok, ShouldBeTrue)
			So(position, ShouldNotBeNil)
			So(position.Holding, ShouldNotBeNil)
			So(position.Holding.Symbol, ShouldEqual, symbol)
			So(position.Holding.Qty, ShouldNotBeNil)
			So(position.Holding.Qty.String(), ShouldEqual, "2")
			So(position.Holding.Status, ShouldEqual, types.PENDING)
			So(position.Holding.Mark, ShouldNotBeNil)
			So(position.Holding.Mark.Sign(), ShouldBeGreaterThan, 0)
			So(position.Holding.Stoploss, ShouldNotBeNil)
			So(position.Holding.Stoploss.Mark, ShouldNotBeNil)
			So(position.Holding.Stoploss.Mark.Sign(), ShouldBeGreaterThan, 0)
			So(position.Holding.Stoploss.Floor, ShouldNotBeNil)
			So(position.Holding.Stoploss.Floor.Sign(), ShouldBeGreaterThan, 0)
		})
	})
}

/*
TestDeskHydrateRecovered proves desk recovery enriches balances-backed open lots
from account-ledger trade history before any UI publish or stoploss sync path.
*/
func TestDeskHydrateRecovered(t *testing.T) {
	previousModel := viper.Get("trading.model")
	t.Cleanup(func() { viper.Set("trading.model", previousModel) })

	Convey("Given paper trade history with a closed lot followed by a new open lot", t, func() {
		viper.Set("trading.model", "paper")
		public := &deskHistoryConn{client: deskNormalizerClient()}
		paper := &deskHistoryConn{history: &krakenmodel.TradesHistory{
			Result: krakenmodel.TradesHistoryResult{Trades: map[string]spot.Trade{
				"closed-buy": {
					Pair:   "BTCUSD",
					Type:   "buy",
					Price:  decimal.NewFromInt64(10),
					Cost:   decimal.NewFromInt64(10),
					Fee:    decimal.NewFromFloat64(0.1),
					Volume: decimal.NewFromInt64(1),
					Time:   decimal.NewFromFloat64(1),
				},
				"closed-sell": {
					Pair:   "BTCUSD",
					Type:   "sell",
					Price:  decimal.NewFromInt64(12),
					Cost:   decimal.NewFromInt64(12),
					Fee:    decimal.NewFromFloat64(0.1),
					Volume: decimal.NewFromInt64(1),
					Time:   decimal.NewFromFloat64(2),
				},
				"open-buy": {
					Pair:   "BTCUSD",
					Type:   "buy",
					Price:  decimal.NewFromInt64(5),
					Cost:   decimal.NewFromInt64(15),
					Fee:    decimal.NewFromFloat64(0.2),
					Volume: decimal.NewFromInt64(3),
					Time:   decimal.NewFromFloat64(3),
				},
			}},
		}}
		api := websocket.NewAPI(context.Background(), public, &deskHistoryConn{}, paper)
		desk := NewDesk(context.Background(), api, nil, nil, nil, config.Fixture().Trading)

		So(api.Initialize(), ShouldBeNil)
		So(desk.hydrateRecovered(), ShouldBeNil)

		Convey("Then the latest open lot economics are recovered by symbol", func() {
			holding := desk.recovered["BTC/USD"]
			So(holding, ShouldNotBeNil)
			So(holding.EntryPrice, ShouldNotBeNil)
			So(holding.EntryPrice.Float64(), ShouldEqual, 5)
			So(holding.EntryFee, ShouldNotBeNil)
			So(holding.EntryFee.Float64(), ShouldAlmostEqual, 0.2, 1e-8)
			So(holding.Stoploss, ShouldNotBeNil)
			So(holding.Stoploss.Entry, ShouldNotBeNil)
			So(holding.Stoploss.Entry.Float64(), ShouldEqual, 5)
		})
	})
}

type deskHistoryConn struct {
	*types.Actor
	client  *spot.WebSocket
	history *krakenmodel.TradesHistory
}

func (conn *deskHistoryConn) Client() *spot.WebSocket { return conn.client }

func (conn *deskHistoryConn) Write(json.Marshaler) error { return nil }

func (conn *deskHistoryConn) Post(string, json.Marshaler) ([]byte, error) { return nil, nil }

func (conn *deskHistoryConn) Close() {}

func (conn *deskHistoryConn) Root() *types.Actor {
	if conn.Actor == nil {
		conn.Actor = types.NewActor(context.Background(), "history-stub", nil)
		conn.Actor.Initialize()
	}

	return conn.Actor
}

func (conn *deskHistoryConn) TradesHistory() (*krakenmodel.TradesHistory, error) {
	return conn.history, nil
}

func deskNormalizerClient() *spot.WebSocket {
	client := spot.NewWebSocket()
	client.REST.Executor = func(request *http.Request) (*http.Response, error) {
		version := request.URL.Query().Get("assetVersion")
		body := `{"error":[],"result":{}}`

		switch request.URL.Path {
		case "/0/public/Assets":
			body = `{"error":[],"result":{"XXBT":{"altname":"XBT"},"ZUSD":{"altname":"USD"}}}`

			if version == "1" {
				body = `{"error":[],"result":{"BTC":{"altname":"XBT"},"USD":{"altname":"USD"}}}`
			}
		case "/0/public/AssetPairs":
			body = `{"error":[],"result":{"XXBTZUSD":{"wsname":"BTC/USD","base":"XXBT","quote":"ZUSD"}}}`

			if version == "1" {
				body = `{"error":[],"result":{"BTC/USD":{"wsname":"BTC/USD","base":"BTC","quote":"USD"}}}`
			}
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	}

	return client
}
