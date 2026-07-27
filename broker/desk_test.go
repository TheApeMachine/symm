package broker

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
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
				"USD": 80595.4943,
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
