package trader

import (
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

func TestUniverseRebalance(t *testing.T) {
	Convey("Given a universe with four ranked candidates, one of them held", t, func() {
		previousTierSize := viper.GetInt("market.universe.trading_tier_size")
		previousQuoteAge := viper.GetDuration("trading.max_quote_age")
		viper.Set("trading.max_quote_age", time.Hour)
		defer viper.Set("market.universe.trading_tier_size", previousTierSize)
		defer viper.Set("trading.max_quote_age", previousQuoteAge)

		now := time.Now().UTC().Format(time.RFC3339Nano)
		tickerJSON := `[
			{"symbol":"AAA/USD","bid":100,"ask":100.01,"bid_qty":1000,"ask_qty":1000,"last":100,"volume":1000000,"vwap":100,"timestamp":"` + now + `"},
			{"symbol":"CCC/USD","bid":50,"ask":50.1,"bid_qty":100,"ask_qty":100,"last":50,"volume":100000,"vwap":50,"timestamp":"` + now + `"},
			{"symbol":"BBB/USD","bid":10,"ask":10.5,"bid_qty":5,"ask_qty":5,"last":10,"volume":1000,"vwap":10,"timestamp":"` + now + `"},
			{"symbol":"DDD/USD","bid":1,"ask":2,"bid_qty":1,"ask_qty":1,"last":1,"volume":1,"vwap":1,"timestamp":"` + now + `"}
		]`
		price := newUniversePrice(t, map[string]kraken.FeeRates{
			"AAA/USD": {Taker: 0.0001},
			"CCC/USD": {Taker: 0.002},
			"BBB/USD": {Taker: 0.01},
			"DDD/USD": {Taker: 0.01},
		}, tickerJSON)

		snapshot := map[string]kraken.TickerData{}
		for _, row := range kraken.NewTickerDataSlice([]byte(tickerJSON)) {
			snapshot[row.Symbol] = row
		}

		desk := broker.NewDesk(&universeFeeConn{}, &universeFeeConn{}, nil)
		desk.SetPrice(price)
		broker.NewExecutions(desk, nil).On([]byte(`[{
			"symbol": "DDD/USD",
			"avg_price": 1.0,
			"exec_type": "snapshot",
			"last_qty": 10,
			"order_status": "filled",
			"position_status": "open",
			"side": "buy"
		}]`))

		pool := testPool()
		public := &writeConn{}
		level3 := &writeConn{}
		instrument := NewInstrument(pool, public, &writeConn{}, level3, nil)
		instrument.status = types.READY

		orderBook := NewOrderBook(25)
		orderBook.Apply(kraken.BookData{
			Symbol: "AAA/USD",
			Type:   "snapshot",
			Bids:   []kraken.BookLevel{{Price: tests.Decimal(t, "100"), Qty: 1}},
			Asks:   []kraken.BookLevel{{Price: tests.Decimal(t, "100.01"), Qty: 1}},
		}, 8)

		level3Book := NewLevel3Book(10)

		Convey("When the trading tier holds two slots", func() {
			viper.Set("market.universe.trading_tier_size", 2)
			universe := NewUniverse(instrument, price, desk, orderBook, level3Book)

			err := universe.Rebalance(snapshot)

			Convey("Then the held symbol and the best-ranked candidate are promoted", func() {
				So(err, ShouldBeNil)
				So(universe.Promoted(), ShouldResemble, []string{"AAA/USD", "DDD/USD"})
				So(public.writes, ShouldEqual, 2)
				So(level3.writes, ShouldEqual, 1)
			})

			Convey("When the top-ranked candidate then goes stale", func() {
				public.writes, level3.writes = 0, 0
				staleSnapshot := map[string]kraken.TickerData{}

				for symbol, row := range snapshot {
					if symbol == "AAA/USD" {
						row.Timestamp = time.Now().Add(-2 * time.Hour)
					}

					staleSnapshot[symbol] = row
				}

				err := universe.Rebalance(staleSnapshot)

				Convey("Then it is replaced by the next best candidate and its book state is evicted", func() {
					So(err, ShouldBeNil)
					So(universe.Promoted(), ShouldResemble, []string{"CCC/USD", "DDD/USD"})
					So(public.writes, ShouldEqual, 4)
					So(level3.writes, ShouldEqual, 2)

					_, _, ok := orderBook.TopOfBook("AAA/USD")
					So(ok, ShouldBeFalse)
				})
			})
		})

		Convey("When the trading tier size is not positive", func() {
			viper.Set("market.universe.trading_tier_size", 0)
			universe := NewUniverse(instrument, price, desk, orderBook, level3Book)

			err := universe.Rebalance(snapshot)

			Convey("Then it reports a validation error", func() {
				So(err, ShouldNotBeNil)
			})
		})
	})
}
