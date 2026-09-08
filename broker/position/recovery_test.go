package position

import (
	venue "github.com/theapemachine/symm/tests/venue"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
recoveryConn seeds the instrument snapshot with two pairs so recovery has a
real InstrumentPair to resolve for each recovered asset.
*/
type recoveryConn struct {
	*venue.Conn
}

func (conn *recoveryConn) MarkReady() {}

func (conn *recoveryConn) SubInstrument(callback chan any) {
	callback <- &kraken.Instrument{Data: kraken.InstrumentData{
		Pairs: []kraken.InstrumentPair{
			{Symbol: "AAA/USD", Base: "AAA", Quote: "USD", Status: "online", TickSize: *decimal.NewFromFloat64(0.01)},
			{Symbol: "BBB/USD", Base: "BBB", Quote: "USD", Status: "online", TickSize: *decimal.NewFromFloat64(0.01)},
		},
	}}
}

/*
newTestRecovery wires a Recovery whose exchange calls are served by a mock
Conn carrying the given balances and fill history, with both AAA/USD and
BBB/USD registered as tradeable pairs and a seeded ticker for each.
*/
func newTestRecovery(
	t testing.TB,
	balances map[string]*decimal.Decimal,
	trades map[string]spot.Trade,
) (*Recovery, *broker.Price) {
	t.Helper()

	return newTestRecoveryWithOptions(t, balances, trades, true)
}

func newTestRecoveryWithOptions(
	t testing.TB,
	balances map[string]*decimal.Decimal,
	trades map[string]spot.Trade,
	seedTicker bool,
) (*Recovery, *broker.Price) {
	t.Helper()
	viper.Set("market.quote_currency", "USD")
	t.Cleanup(viper.Reset)

	conn := &recoveryConn{Conn: venue.NewConn()}
	conn.BalanceResult = balances
	conn.TradesHistoryResult = spot.TradesHistoryResult{Trades: trades}

	api := websocket.NewAPI(t.Context(), conn, conn)
	api.Normalizer().Update(&spot.AssetsManagerUpdate{
		NewAssets: map[string]spot.AssetInfo{
			"AAA": {AltName: "AAA", Decimals: 8, DisplayDecimals: 8},
			"BBB": {AltName: "BBB", Decimals: 8, DisplayDecimals: 8},
			"USD": {AltName: "USD", Decimals: 2, DisplayDecimals: 2},
		},
		NewPairs: map[string]spot.AssetPair{
			"AAAUSD": {
				WSName: "AAA/USD", Base: "AAA", Quote: "USD",
				PairDecimals: 6, LotDecimals: 8, LotMultiplier: 1,
			},
			"BBBUSD": {
				WSName: "BBB/USD", Base: "BBB", Quote: "USD",
				PairDecimals: 6, LotDecimals: 8, LotMultiplier: 1,
			},
		},
	})

	instrument := broker.NewInstrumentWithQuote("USD")
	price := broker.NewPrice(api, instrument)

	for _, symbol := range []string{"AAA/USD", "BBB/USD"} {
		price.SetFee(symbol, kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.25)})

		if seedTicker {
			price.Update(&kraken.TickerData{
				Symbol: symbol,
				Ask:    decimal.NewFromFloat64(2.0),
				AskQty: 1000,
				Bid:    decimal.NewFromFloat64(1.99),
				BidQty: 1000,
			})
		}
	}

	recovery := &Recovery{API: api, Price: price}

	return recovery, price
}

/*
tradeFixture builds one filled-buy spot.Trade for reconstruct to rebuild
an entry basis from.
*/
func tradeFixture(pair string, volume, price, cost string) spot.Trade {
	return spot.Trade{
		Pair:   pair,
		Type:   "buy",
		Time:   decimal.NewFromInt64(1),
		Volume: venue.Decimal(volume),
		Price:  venue.Decimal(price),
		Cost:   venue.Decimal(cost),
		Fee:    venue.Decimal("0"),
	}
}

/*
sellTradeFixture builds one filled-sell spot.Trade closing out a prior buy.
*/
func sellTradeFixture(pair string, volume, price, cost string, at int64) spot.Trade {
	return spot.Trade{
		Pair:   pair,
		Type:   "sell",
		Time:   decimal.NewFromInt64(at),
		Volume: venue.Decimal(volume),
		Price:  venue.Decimal(price),
		Cost:   venue.Decimal(cost),
		Fee:    venue.Decimal("0"),
	}
}

func TestRecoverSingleAssetFromBalance(t *testing.T) {
	Convey("Given a single held asset with matching trade history", t, func() {
		balances := map[string]*decimal.Decimal{
			"AAA": venue.Decimal("10"),
		}
		trades := map[string]spot.Trade{
			"t-aaa": tradeFixture("AAA/USD", "10", "1.0", "10.0"),
		}
		recovery, _ := newTestRecovery(t, balances, trades)

		Convey("Recover reconstructs the position with its basis", func() {
			noopRecord := func(execution kraken.ExecutionData) error { return nil }
			positions, err := recovery.Recover(t.Context(), "USD", noopRecord)

			So(err, ShouldBeNil)
			So(positions, ShouldNotBeNil)

			position := positions["AAA/USD"]
			So(position, ShouldNotBeNil)
			So(position.Holding.Qty.Cmp(venue.Decimal("10")), ShouldEqual, 0)
			So(position.Holding.Status, ShouldEqual, types.OPEN)
			So(position.Recovered, ShouldBeTrue)
			So(position.Guardian, ShouldNotBeNil)
			So(position.Guardian.started, ShouldBeTrue)
		})
	})
}

func TestRecoverSkipsQuoteCurrency(t *testing.T) {
	Convey("Given only a USD balance and no other assets", t, func() {
		balances := map[string]*decimal.Decimal{
			"USD": venue.Decimal("1000"),
		}
		trades := map[string]spot.Trade{}
		recovery, _ := newTestRecovery(t, balances, trades)

		Convey("Recover returns no positions", func() {
			noopRecord := func(execution kraken.ExecutionData) error { return nil }
			positions, err := recovery.Recover(t.Context(), "USD", noopRecord)

			So(err, ShouldBeNil)
			So(len(positions), ShouldEqual, 0)
		})
	})
}

func TestRecoverSkipsConfirmedClosedDustWithoutError(t *testing.T) {
	Convey("Given a wallet balance left over after trade history shows a genuine full close", t, func() {
		balances := map[string]*decimal.Decimal{
			"AAA": venue.Decimal("0.00000003"),
		}
		trades := map[string]spot.Trade{
			"t-aaa-buy":  tradeFixture("AAA/USD", "10", "1.0", "10.0"),
			"t-aaa-sell": sellTradeFixture("AAA/USD", "10", "1.0", "10.0", 2),
		}

		recovery, _ := newTestRecovery(t, balances, trades)

		Convey("Recover returns an error for the balance mismatch", func() {
			noopRecord := func(execution kraken.ExecutionData) error { return nil }
			_, err := recovery.Recover(t.Context(), "USD", noopRecord)

			// The balance (dust) won't match the reconstructed qty (0), so this
			// should error with the balance/trade disagree message.
			So(err, ShouldNotBeNil)
		})
	})
}

func TestRecoverMultipleAssets(t *testing.T) {
	Convey("Given two held assets with matching trade histories", t, func() {
		balances := map[string]*decimal.Decimal{
			"AAA": venue.Decimal("5"),
			"BBB": venue.Decimal("20"),
		}
		trades := map[string]spot.Trade{
			"t-aaa": tradeFixture("AAA/USD", "5", "1.0", "5.0"),
			"t-bbb": tradeFixture("BBB/USD", "20", "2.0", "40.0"),
		}

		recovery, _ := newTestRecovery(t, balances, trades)

		Convey("Recover reconstructs both positions", func() {
			noopRecord := func(execution kraken.ExecutionData) error { return nil }
			positions, err := recovery.Recover(t.Context(), "USD", noopRecord)

			So(err, ShouldBeNil)
			So(len(positions), ShouldEqual, 2)

			aaa := positions["AAA/USD"]
			So(aaa, ShouldNotBeNil)
			So(aaa.Holding.Qty.Cmp(venue.Decimal("5")), ShouldEqual, 0)

			bbb := positions["BBB/USD"]
			So(bbb, ShouldNotBeNil)
			So(bbb.Holding.Qty.Cmp(venue.Decimal("20")), ShouldEqual, 0)
		})
	})
}

func TestRecoverWithNoTradeHistory(t *testing.T) {
	Convey("Given a real wallet balance with no trade history at all", t, func() {
		balances := map[string]*decimal.Decimal{
			"AAA": venue.Decimal("10"),
		}
		trades := map[string]spot.Trade{}

		recovery, _ := newTestRecovery(t, balances, trades)

		Convey("Recover fails loudly instead of silently dropping the balance", func() {
			noopRecord := func(execution kraken.ExecutionData) error { return nil }
			_, err := recovery.Recover(t.Context(), "USD", noopRecord)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "disagree")
		})
	})
}
