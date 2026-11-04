package broker

import (
	"context"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/runtime"
)

func TestDecodeLotBareAndWrapped(t *testing.T) {
	Convey("decodeLot reads a bare position", t, func() {
		raw, err := sonic.Marshal(&Position{
			ID:       "lot-1",
			Symbol:   "ETH/USD",
			OpenedAt: time.Now().UTC(),
		})
		So(err, ShouldBeNil)

		got := decodeLot(raw)

		So(got, ShouldNotBeNil)
		So(got.Symbol, ShouldEqual, "ETH/USD")
		So(got.ID, ShouldEqual, "lot-1")
	})

	Convey("decodeLot peels a nested position envelope", t, func() {
		raw := []byte(`{"epoch":1,"tick":2,"timestamp":3,"position":{"id":"lot-2","symbol":"AAVE/USD","opened_at":"2026-11-03T15:07:12.627877Z"}}`)

		got := decodeLot(raw)

		So(got, ShouldNotBeNil)
		So(got.Symbol, ShouldEqual, "AAVE/USD")
		So(got.ID, ShouldEqual, "lot-2")
	})
}

func TestRestoreNilPnLPct(t *testing.T) {
	Convey("Restore survives a lot with no stoploss and no pnl_pct", t, func() {
		cache := &Cache{
			System:  runtime.NewSystem(context.Background(), "cache"),
			tierKey: "virtual",
		}

		raw := []byte(`{"position":{"id":"lot-3","symbol":"ADA/USD","book":"virtual","opened_at":"2026-11-03T15:07:12.627877Z","entry_order":{"cl_ord_id":"x","ordertype":"market","type":"buy","volume":"1"}}}`)

		got := cache.Restore(raw)

		So(got, ShouldNotBeNil)
		So(got.Symbol, ShouldEqual, "ADA/USD")
		So(got.Stoploss, ShouldNotBeNil)
	})
}

func TestRestoreClearsClosedAt(t *testing.T) {
	Convey("Open-prefix Restore clears ClosedAt so settle can finish", t, func() {
		cache := &Cache{
			System:  runtime.NewSystem(context.Background(), "cache"),
			tierKey: "virtual",
		}

		raw := []byte(`{"id":"lot-4","symbol":"XRP/USD","book":"virtual","opened_at":"2026-11-03T15:07:12.627877Z","closed_at":"2026-11-03T16:00:00Z","entry_order":{"cl_ord_id":"x","ordertype":"market","type":"buy","volume":"11.73"}}`)

		got := cache.Restore(raw)

		So(got, ShouldNotBeNil)
		So(got.ClosedAt, ShouldBeNil)
	})
}

func TestHasKeepsOpenClosedAt(t *testing.T) {
	Convey("Has does not reclaim a ClosedAt lot out of open", t, func() {
		now := time.Now().UTC()
		cache := &Cache{
			System:  runtime.NewSystem(context.Background(), "cache"),
			tierKey: "virtual",
			positions: map[string]map[string]*Position{
				"open": {
					"XRP/USD": {Symbol: "XRP/USD", ClosedAt: &now},
				},
				"closed": {},
			},
		}

		So(cache.Has("XRP/USD"), ShouldBeTrue)
		So(cache.positions["open"]["XRP/USD"], ShouldNotBeNil)
		So(cache.positions["closed"]["XRP/USD"], ShouldBeNil)
	})
}

func TestRestoreClosedKeepsClosedAt(t *testing.T) {
	Convey("Closed-prefix restore keeps the settle time", t, func() {
		cache := &Cache{
			System:  runtime.NewSystem(context.Background(), "cache"),
			tierKey: "real",
		}

		raw := []byte(`{"id":"lot-5","symbol":"SOL/USD","book":"real","opened_at":"2026-11-03T15:07:12.627877Z","closed_at":"2026-11-03T16:00:00Z"}`)

		got := cache.restore(raw, false)

		So(got, ShouldNotBeNil)
		So(got.ClosedAt, ShouldNotBeNil)
		So(got.ClosedAt.UTC().Format(time.RFC3339), ShouldEqual, "2026-11-03T16:00:00Z")
	})
}
