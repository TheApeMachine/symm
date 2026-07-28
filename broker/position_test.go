package broker

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests"
	executionfixture "github.com/theapemachine/symm/tests/fixtures/execution"
	orderackfixture "github.com/theapemachine/symm/tests/fixtures/orderack"
	"github.com/theapemachine/symm/tests/mockapi"
	"github.com/theapemachine/symm/types"
)

/*
TestPositionCorrelatesPrivateFrames proves one venue ack/fill only mutates the
matching position and keeps entry economics at unit-price scale.
*/
func TestPositionCorrelatesPrivateFrames(t *testing.T) {
	previousModel := viper.Get("trading.model")
	t.Cleanup(func() { viper.Set("trading.model", previousModel) })

	Convey("Given two open positions sharing one account actor", t, func() {
		viper.Set("trading.model", "live")
		market := tests.NewMarket(t.Context(), 2)
		So(market.Bootstrap(), ShouldBeNil)
		Reset(market.Close)

		api := websocket.NewAPI(t.Context(), market.Public, market.Private, market.Paper)
		price := NewPrice(api)
		balance := NewBalance(api, nil, config.Fixture().Market)

		So(api.Initialize(), ShouldBeNil)
		So(price.Initialize(), ShouldBeNil)
		So(price.RememberFee(market.Symbols[0], kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.26)}), ShouldBeNil)
		So(price.RememberFee(market.Symbols[1], kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.26)}), ShouldBeNil)

		entryQtyOne := decimal.NewFromInt64(2)
		entryQtyTwo := decimal.NewFromInt64(1)
		pairOne := kraken.InstrumentPair{Symbol: market.Symbols[0], Quote: "USD", CostPrecision: 5}
		pairTwo := kraken.InstrumentPair{Symbol: market.Symbols[1], Quote: "USD", CostPrecision: 5}
		positionOne := NewPosition(
			t.Context(),
			api,
			nil,
			nil,
			price,
			balance,
			pairOne,
			entryQtyOne,
			market.Public.Root(),
			api.Account().Root(),
		)
		positionTwo := NewPosition(
			t.Context(),
			api,
			nil,
			nil,
			price,
			balance,
			pairTwo,
			entryQtyTwo,
			market.Public.Root(),
			api.Account().Root(),
		)

		So(market.Private.Queue("add_order", orderackfixture.Frame(orderackfixture.Options{
			ReqID:   positionOne.EntryOrder.ReqID,
			OrderID: "LIVE-ORDER-1",
			Success: true,
		})), ShouldBeNil)
		So(market.Private.Queue("executions", executionfixture.Frame(executionfixture.Options{
			OrderID:     "LIVE-ORDER-1",
			ExecID:      "LIVE-EXEC-1",
			Symbol:      market.Symbols[0],
			Side:        "buy",
			LastQty:     "2.00000000",
			LastPrice:   "10.00000000",
			Cost:        "20.00000000",
			OrderStatus: "filled",
			OrderType:   "market",
			ExecType:    "trade",
			CumQty:      "2.00000000",
			CumCost:     "20.00000000",
			AvgPrice:    "10.00000000",
			FeeUsdEquiv: "0.10000000",
		})), ShouldBeNil)
		So(market.Private.Drain(), ShouldBeNil)

		deadline := time.Now().Add(time.Second)

		for time.Now().Before(deadline) {
			if positionOne.OrderID == "LIVE-ORDER-1" && positionOne.Holding.EntryPrice != nil {
				break
			}

			time.Sleep(time.Millisecond)
		}

		Convey("Then only the matching position binds the order and fill", func() {
			So(positionOne.OrderID, ShouldEqual, "LIVE-ORDER-1")
			So(positionTwo.OrderID, ShouldEqual, "")
			So(positionOne.Fills, ShouldHaveLength, 1)
			So(positionTwo.Fills, ShouldBeEmpty)
		})

		Convey("Then the buy fill keeps entry economics at price-per-unit scale", func() {
			So(positionOne.Holding.EntryPrice, ShouldNotBeNil)
			So(positionOne.Holding.EntryPrice.Float64(), ShouldEqual, 10)
			So(positionOne.Holding.Stoploss, ShouldNotBeNil)
			So(positionOne.Holding.Stoploss.Mark.Float64(), ShouldEqual, 10)
		})
	})
}

/*
TestPositionExitWaitsForFill proves a submitted sell does not disappear from the
desk lifecycle before the venue confirms the exit execution.
*/
func TestPositionExitWaitsForFill(t *testing.T) {
	previousModel := viper.Get("trading.model")
	t.Cleanup(func() { viper.Set("trading.model", previousModel) })

	Convey("Given an open position exiting through the shared account actor", t, func() {
		viper.Set("trading.model", "live")
		market := tests.NewMarket(t.Context(), 1)
		So(market.Bootstrap(), ShouldBeNil)
		Reset(market.Close)
		So(market.Private.EnablePaper(mockapi.PaperOptions{
			Quote: func(symbol string) (bid, bidQty, ask, askQty float64, exists bool) {
				return 10, 5, 10.1, 5, symbol == market.Symbols[0]
			},
			Now: market.Now,
			Balances: map[string]float64{
				"USD":  1000,
				"SIM1": 1,
			},
			MakerFee: 0.0016,
			TakerFee: 0.0026,
		}), ShouldBeNil)

		api := websocket.NewAPI(t.Context(), market.Public, market.Private, market.Paper)
		price := NewPrice(api)
		balance := NewBalance(api, nil, config.Fixture().Market)

		So(api.Initialize(), ShouldBeNil)
		So(price.Initialize(), ShouldBeNil)
		So(price.RememberFee(market.Symbols[0], kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.26)}), ShouldBeNil)

		pair := kraken.InstrumentPair{Symbol: market.Symbols[0], Quote: "USD", CostPrecision: 5}
		position := NewPosition(
			t.Context(),
			api,
			nil,
			nil,
			price,
			balance,
			pair,
			decimal.NewFromInt64(1),
			market.Public.Root(),
			api.Account().Root(),
		)
		position.OrderID = "ENTRY-1"
		position.Status = types.FILLED
		position.Holding.Status = types.FILLED

		So(position.Exit(), ShouldBeNil)

		Convey("Then the lot stays open until a matching sell execution arrives", func() {
			So(position.Status, ShouldEqual, types.PENDING)
			So(position.Holding.Status, ShouldEqual, types.PENDING)
		})

		So(market.Private.Queue("add_order", orderackfixture.Frame(orderackfixture.Options{
			ReqID:   position.ExitOrder.ReqID,
			OrderID: "EXIT-1",
			Success: true,
		})), ShouldBeNil)
		So(market.Private.Queue("executions", executionfixture.Frame(executionfixture.Options{
			OrderID:     "EXIT-1",
			ExecID:      "EXIT-EXEC-1",
			Symbol:      market.Symbols[0],
			Side:        "sell",
			LastQty:     "1.00000000",
			LastPrice:   "10.00000000",
			Cost:        "10.00000000",
			OrderStatus: "filled",
			OrderType:   "market",
			ExecType:    "trade",
			CumQty:      "1.00000000",
			CumCost:     "10.00000000",
			AvgPrice:    "10.00000000",
			FeeUsdEquiv: "0.10000000",
		})), ShouldBeNil)
		So(market.Private.Drain(), ShouldBeNil)

		deadline := time.Now().Add(time.Second)

		for time.Now().Before(deadline) {
			if position.Status == types.CLOSED {
				break
			}

			time.Sleep(time.Millisecond)
		}

		Convey("Then the lot closes only after that sell fill", func() {
			So(position.Status, ShouldEqual, types.CLOSED)
			So(position.Holding.Status, ShouldEqual, types.CLOSED)
		})
	})
}

/*
TestPositionTickerPublishesThroughStoploss proves ticker marks do not publish
directly from Position. Stoploss owns the ticker-side publish callback so the UI
only sees snapshots after the regulator has absorbed the fresh mark.
*/
func TestPositionTickerPublishesThroughStoploss(t *testing.T) {
	Convey("Given a position mark update on a live ticker", t, func() {
		published := 0
		price := NewPrice(nil)
		symbol := "SIM1/USD"
		bid := decimal.NewFromFloat64(101.5)
		price.tickers[symbol] = &kraken.TickerData{
			Symbol: symbol,
			Bid:    bid,
			Last:   bid,
		}

		holding := types.NewHolding(
			t.Context(),
			symbol,
			decimal.NewFromInt64(1),
			decimal.NewFromFloat64(100),
			func() error { return nil },
			func() { published++ },
			nil,
		)
		position := &Position{
			price:   price,
			pair:    kraken.InstrumentPair{Symbol: symbol, Quote: "USD", CostPrecision: 5},
			Holding: holding,
		}

		position.onTicker(&kraken.Ticker{Data: []kraken.TickerData{{
			Symbol: symbol,
			Bid:    bid,
		}}})

		Convey("Then stoploss publishes the coherent snapshot and peak keeps up with mark", func() {
			So(position.Holding.Mark, ShouldNotBeNil)
			So(position.Holding.Mark.Cmp(bid), ShouldEqual, 0)
			So(position.Holding.Stoploss.Peak, ShouldNotBeNil)
			So(position.Holding.Stoploss.Peak.Cmp(position.Holding.Mark) >= 0, ShouldBeTrue)
			So(published, ShouldEqual, 1)
		})
	})
}

/*
TestDeskUsesSimulatedPrivateLiveTransport proves the fixture market can drive
the live-model private path end to end without touching a real venue socket.
*/
func TestDeskUsesSimulatedPrivateLiveTransport(t *testing.T) {
	previousModel := viper.Get("trading.model")
	t.Cleanup(func() { viper.Set("trading.model", previousModel) })

	Convey("Given the live trading model against the simulated market", t, func() {
		viper.Set("trading.model", "live")
		market := tests.NewMarket(t.Context(), 1)
		cfg := config.Fixture()
		cfg.Market.L3Enabled = false
		ui := make(chan []byte, 1024)
		So(market.Bootstrap(), ShouldBeNil)
		Reset(market.Close)

		api := websocket.NewAPI(t.Context(), market.Public, market.Private, market.Paper)
		price := NewPrice(api)
		instrument := NewInstrument(api, price, ui, cfg.Market)
		balance := NewBalance(api, ui, cfg.Market)
		desk := NewDesk(t.Context(), api, instrument, price, balance, cfg.Trading)

		So(api.Initialize(), ShouldBeNil)
		So(price.Initialize(), ShouldBeNil)
		So(balance.Initialize(), ShouldBeNil)
		So(desk.Initialize(market.Public.Root(), api.Account().Root()), ShouldBeNil)
		So(instrument.Initialize(), ShouldBeNil)

		Convey("Then private subscriptions and order flow stay on the simulated private Conn", func() {
			So(market.Private.Subscribed("balances"), ShouldBeTrue)
			So(market.Private.Subscribed("executions"), ShouldBeTrue)
			So(market.Paper.Subscribed("balances"), ShouldBeFalse)
			So(market.Paper.Subscribed("executions"), ShouldBeFalse)

			position, err := desk.Buy(market.Symbols[0], decimal.NewFromInt64(1), false)
			So(err, ShouldBeNil)
			So(market.Private.Drain(), ShouldBeNil)
			So(position.OrderID, ShouldNotEqual, "")
			So(position.Fills, ShouldHaveLength, 1)
		})
	})
}
