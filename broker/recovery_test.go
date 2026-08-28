package broker

import (
	"sync"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/tests/mock"
	"github.com/theapemachine/symm/types"
)

/*
recoveryConn seeds the instrument snapshot with two pairs so recovery has a
real InstrumentPair to resolve for each recovered asset.
*/
type recoveryConn struct {
	*mock.Conn
}

func (conn *recoveryConn) SubInstrument(callback chan any) {
	callback <- &kraken.Instrument{Data: kraken.InstrumentData{
		Pairs: []kraken.InstrumentPair{
			{Symbol: "AAA/USD", Base: "AAA", Quote: "USD", Status: "online"},
			{Symbol: "BBB/USD", Base: "BBB", Quote: "USD", Status: "online"},
		},
	}}
}

/*
newTestRecovery wires a Recovery whose exchange calls are served by a mock
Conn carrying the given balances and fill history, with both AAA/USD and
BBB/USD registered as tradeable pairs.
*/
func newTestRecovery(
	t testing.TB,
	balances map[string]*decimal.Decimal,
	trades map[string]spot.Trade,
) (*Recovery, *sync.Map) {
	t.Helper()
	viper.Set("market.quote_currency", "USD")
	t.Cleanup(viper.Reset)

	conn := &recoveryConn{Conn: mock.NewConn()}
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

	bus := runtime.NewWorkspace(t.Context())
	bus.Share("api", api, "")
	bus.Share("price", newTestPrice(t, api), "")
	instrument := NewInstrument(t.Context(), bus)
	price := newTestPrice(t, api)

	storePath := t.TempDir() + "/recovery.sqlite"
	store, err := NewPositionStore(storePath)
	if err != nil {
		t.Fatalf("failed to open position store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	positions := &sync.Map{}
	recovery := NewRecovery(
		t.Context(), api, bus, instrument, price, nil, nil, store, positions,
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
A single asset's recovery failure must never cost every other asset its
tracking and protection. balances is a plain Go map, so Recover used to
iterate it in random order and RETURN on the very first recoverAsset error —
whichever assets the random order hadn't reached yet were silently dropped:
genuinely open, unprotected, and invisible in the UI, exactly the scenario the
user was worried about after a restart. This proves recovery now attempts
every asset regardless of an earlier failure.
*/
func TestRecoverContinuesPastOneAssetFailure(t *testing.T) {
	Convey("Given two open assets where one has no stored stoploss to restore", t, func() {
		balances := map[string]*decimal.Decimal{
			"AAA": mustDecimal("10"),
			"BBB": mustDecimal("20"),
		}
		trades := map[string]spot.Trade{
			"t-aaa": tradeFixture("AAA/USD", "10", "1.0", "10.0"),
			"t-bbb": tradeFixture("BBB/USD", "20", "2.0", "40.0"),
		}

		recovery, positions := newTestRecovery(t, balances, trades)

		// BBB gets a persisted stoploss row (recoverable); AAA does not, so
		// its recoverAsset call hits the "stored stoploss required" NotFound
		// path deliberately.
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
		}
		if err := recovery.store.Save(bbbStoploss); err != nil {
			t.Fatalf("failed to seed BBB stoploss: %v", err)
		}

		Convey("Recover reports the AAA failure but still restores BBB", func() {
			err := recovery.Recover()

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "AAA")

			_, aaaRestored := positions.Load("AAA/USD")
			_, bbbRestored := positions.Load("BBB/USD")

			So(aaaRestored, ShouldBeFalse)
			So(bbbRestored, ShouldBeTrue)
		})
	})
}
