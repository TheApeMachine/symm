package broker

import (
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
recoveryConn seeds the instrument snapshot with two pairs so recovery has a
real InstrumentPair to resolve for each recovered asset.
*/
type recoveryConn struct {
	*mockConn
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
) (*Recovery, *sync.Map) {
	t.Helper()

	return newTestRecoveryWithOptions(t, balances, trades, true)
}

/*
newTestRecoveryWithOptions is newTestRecovery with control over whether a
ticker is seeded, so a test can exercise synthesizeStoploss's no-live-quote
fallback path — the shape recovery actually runs under at boot, before the
instrument subscription has delivered any market data.
*/
func newTestRecoveryWithOptions(
	t testing.TB,
	balances map[string]*decimal.Decimal,
	trades map[string]spot.Trade,
	seedTicker bool,
) (*Recovery, *sync.Map) {
	t.Helper()
	viper.Set("market.quote_currency", "USD")
	t.Cleanup(viper.Reset)

	conn := &recoveryConn{mockConn: newMockConn()}
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

	price := newTestPrice(t, api)
	instrument := NewInstrument(api, price)

	for _, symbol := range []string{"AAA/USD", "BBB/USD"} {
		price.fees.Store(symbol, kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.25)})

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

	storePath := t.TempDir() + "/recovery.sqlite"
	store, err := NewPositionStore(
		storePath, testPositionStoreQueueDepth, testPositionStoreBatchSize,
	)
	if err != nil {
		t.Fatalf("failed to open position store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	positions := &sync.Map{}
	recovery := NewRecovery(
		t.Context(), api, instrument, price, nil, store, positions,
	)

	return recovery, positions
}

/*
tradeFixture builds one filled-buy spot.Trade for recoverBasis to reconstruct
an entry basis from.
*/
func tradeFixture(pair string, volume, price, cost string) spot.Trade {
	return spot.Trade{
		Pair:   pair,
		Type:   "buy",
		Time:   decimal.NewFromInt64(1),
		Volume: mustDecimal(volume),
		Price:  mustDecimal(price),
		Cost:   mustDecimal(cost),
		Fee:    mustDecimal("0"),
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
		Volume: mustDecimal(volume),
		Price:  mustDecimal(price),
		Cost:   mustDecimal(cost),
		Fee:    mustDecimal("0"),
	}
}

/*
A single asset's recovery failure must never cost every other asset its
tracking and protection. balances is a plain Go map, so Recover used to
iterate it in random order and RETURN on the very first recoverAsset error —
whichever assets the random order hadn't reached yet were silently dropped:
genuinely open, unprotected, and invisible in the UI, exactly the scenario the
user was worried about after a restart. This proves recovery now attempts
every asset regardless of an earlier failure.
*/
func TestRecoverContinuesPastOneAssetFailure(t *testing.T) {
	Convey("Given two open assets where one has no tradeable instrument pair", t, func() {
		balances := map[string]*decimal.Decimal{
			"AAA": mustDecimal("10"),
			"BBB": mustDecimal("20"),
			"CCC": mustDecimal("30"),
		}
		trades := map[string]spot.Trade{
			"t-aaa": tradeFixture("AAA/USD", "10", "1.0", "10.0"),
			"t-bbb": tradeFixture("BBB/USD", "20", "2.0", "40.0"),
			"t-ccc": tradeFixture("CCC/USD", "30", "1.0", "30.0"),
		}

		recovery, positions := newTestRecovery(t, balances, trades)

		// EntryAt must match what recoverBasis derives from the BBB trade
		// fixture's Time (decimal 1) — time.Unix(1, 0).UTC() — since Load
		// now keys on (symbol, entry_at) rather than symbol alone.
		bbbEntryAt := time.Unix(1, 0).UTC()
		bbbStoploss := &types.Stoploss{
			Symbol:        "BBB/USD",
			Status:        types.ARMED,
			TickSize:      mustDecimal("0.01"),
			TrailDistance: mustDecimal("0.1"),
			Floor:         mustDecimal("1.5"),
			Mark:          mustDecimal("2.0"),
			Peak:          mustDecimal("2.2"),
			ProfitLine:    mustDecimal("1.8"),
			ArmAt:         mustDecimal("1.7"),
			LockFloor:     mustDecimal("1.6"),
			EntryAt:       &bbbEntryAt,
		}
		if err := recovery.store.Save(bbbStoploss); err != nil {
			t.Fatalf("failed to seed BBB stoploss: %v", err)
		}

		// CCC has no registered instrument pair (recoveryConn.SubInstrument
		// only publishes AAA/USD and BBB/USD), so its recoverAsset call hits
		// the "no instrument pair" NotFound path deliberately.

		Convey("Recover reports the CCC failure but still restores AAA and BBB", func() {
			err := recovery.Recover()

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "CCC")

			_, aaaRestored := positions.Load("AAA/USD")
			_, bbbRestored := positions.Load("BBB/USD")
			_, cccRestored := positions.Load("CCC/USD")

			So(aaaRestored, ShouldBeTrue)
			So(bbbRestored, ShouldBeTrue)
			So(cccRestored, ShouldBeFalse)
		})
	})
}

/*
A wallet balance with a real, unmatched buy in trade history is a genuinely
open position even when the local SQLite stoploss row backing it is gone —
the process can die between a fill landing and its execution frame persisting
that row. Recovery used to treat the missing row as fatal and drop the
position entirely: the exchange still shows the inventory, but the running
system loses all track of it, unprotected and invisible in the UI. This
proves the position is instead recovered with protection rebuilt from current
market geometry.
*/
func TestRecoverSynthesizesStoplossWhenStoreRowMissing(t *testing.T) {
	Convey("Given an open asset whose stoploss row is missing from the store", t, func() {
		balances := map[string]*decimal.Decimal{
			"AAA": mustDecimal("10"),
		}
		trades := map[string]spot.Trade{
			"t-aaa": tradeFixture("AAA/USD", "10", "1.0", "10.0"),
		}

		recovery, positions := newTestRecovery(t, balances, trades)

		Convey("Recover rebuilds protection and restores the position", func() {
			err := recovery.Recover()

			So(err, ShouldBeNil)

			value, restored := positions.Load("AAA/USD")
			So(restored, ShouldBeTrue)

			position, ok := value.(*Position)
			So(ok, ShouldBeTrue)
			So(position.Holding.Stoploss, ShouldNotBeNil)
			So(position.Holding.Stoploss.Status, ShouldEqual, types.ARMED)
			So(position.Holding.Stoploss.Floor, ShouldNotBeNil)
			So(position.Holding.Stoploss.Floor.Sign(), ShouldBeGreaterThan, 0)
		})
	})
}

/*
Recover runs before the instrument subscription has delivered any ticker or
book frame for the symbols it is trying to protect — that subscription
happens much later in boot, batched across the whole tradeable universe. A
position whose stored stoploss row is missing must still be recoverable in
that window: synthesizeStoploss cannot require a live quote it has no way to
have yet, only the wallet balance, trade history, and the venue's own tick
size.
*/
func TestRecoverSynthesizesStoplossWithoutLiveQuote(t *testing.T) {
	Convey("Given an open asset with no ticker or book data available yet", t, func() {
		balances := map[string]*decimal.Decimal{
			"AAA": mustDecimal("10"),
		}
		trades := map[string]spot.Trade{
			"t-aaa": tradeFixture("AAA/USD", "10", "1.0", "10.0"),
		}

		recovery, positions := newTestRecoveryWithOptions(t, balances, trades, false)

		Convey("Recover still rebuilds protection from entry price and tick size alone", func() {
			err := recovery.Recover()

			So(err, ShouldBeNil)

			value, restored := positions.Load("AAA/USD")
			So(restored, ShouldBeTrue)

			position, ok := value.(*Position)
			So(ok, ShouldBeTrue)
			So(position.Holding.Stoploss, ShouldNotBeNil)
			So(position.Holding.Stoploss.Status, ShouldEqual, types.ARMED)
			So(position.Holding.Stoploss.Floor, ShouldNotBeNil)
			So(position.Holding.Stoploss.Floor.Sign(), ShouldBeGreaterThan, 0)
			// With no live book, mark starts at the known entry price rather
			// than an unavailable BestBid.
			So(position.Holding.Stoploss.Mark.Cmp(mustDecimal("1.0")), ShouldEqual, 0)
		})
	})
}

/*
A wallet balance that fully round-trips to zero across its own trade history
(a completed buy-then-sell pair, left with only floating-point residue below
the venue's own lot granularity) is a confirmed-closed position, not an
unexplained one. Recovery must not report this as a failure — there is
nothing left to recover — but it also must not be confused with a real
balance recoverBasis simply failed to explain, which AGENTS.md's "no silent
failures" rule requires to surface loudly instead of being skipped.
*/
func TestRecoverSkipsConfirmedClosedDustWithoutError(t *testing.T) {
	Convey("Given a wallet balance left over after trade history shows a genuine full close", t, func() {
		// The sell fully closes the buy in the reported trade rows
		// (recoverBasis's own full-close reset fires), exactly as it would
		// for a real completed round trip. The wallet still carries a tiny
		// nonzero balance because venue settlement accumulates its own
		// rounding independent of what the trade rows sum to — that
		// leftover is confirmed-closed dust, not an unexplained balance.
		balances := map[string]*decimal.Decimal{
			"AAA": mustDecimal("0.00000003"),
		}
		trades := map[string]spot.Trade{
			"t-aaa-buy":  tradeFixture("AAA/USD", "10", "1.0", "10.0"),
			"t-aaa-sell": sellTradeFixture("AAA/USD", "10", "1.0", "10.0", 2),
		}

		recovery, positions := newTestRecovery(t, balances, trades)

		Convey("Recover succeeds and leaves the dust asset untracked", func() {
			err := recovery.Recover()

			So(err, ShouldBeNil)

			_, restored := positions.Load("AAA/USD")
			So(restored, ShouldBeFalse)
		})
	})

	Convey("Given a real wallet balance with no trade history at all", t, func() {
		balances := map[string]*decimal.Decimal{
			"AAA": mustDecimal("10"),
		}
		trades := map[string]spot.Trade{}

		recovery, _ := newTestRecovery(t, balances, trades)

		Convey("Recover fails loudly instead of silently dropping the balance", func() {
			err := recovery.Recover()

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "AAA")
			So(err.Error(), ShouldContainSubstring, "not accounted for")
		})
	})
}
