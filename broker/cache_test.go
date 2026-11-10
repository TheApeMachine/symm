package broker

import (
	"context"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/runtime"
)

func TestRestore(t *testing.T) {
	Convey("Restore unmarshals a bare position and rebinds collaborators", t, func() {
		cache := &Cache{
			System:  runtime.NewSystem(context.Background(), "cache"),
			tierKey: "virtual",
		}

		raw, err := sonic.Marshal(&Position{
			ID:       "lot-1",
			Symbol:   "ETH/USD",
			OpenedAt: time.Now().UTC(),
		})
		So(err, ShouldBeNil)

		got := cache.Restore(raw)

		So(got, ShouldNotBeNil)
		So(got.Symbol, ShouldEqual, "ETH/USD")
		So(got.ID, ShouldEqual, "lot-1")
		So(got.TierKey, ShouldEqual, "virtual")
	})
}

func TestRestoreNilPnLPct(t *testing.T) {
	Convey("Restore survives a lot with no stoploss and no pnl_pct", t, func() {
		cache := &Cache{
			System:  runtime.NewSystem(context.Background(), "cache"),
			tierKey: "virtual",
		}

		raw := []byte(`{"id":"lot-3","symbol":"ADA/USD","tier_key":"virtual","opened_at":"2026-11-03T15:07:12.627877Z","entry_order":{"cl_ord_id":"x","ordertype":"market","type":"buy","volume":"1"}}`)

		got := cache.Restore(raw)

		So(got, ShouldNotBeNil)
		So(got.Symbol, ShouldEqual, "ADA/USD")
	})
}

func TestRestoreKeepsClosedAt(t *testing.T) {
	Convey("Restore leaves ClosedAt alone — open load clears it", t, func() {
		cache := &Cache{
			System:  runtime.NewSystem(context.Background(), "cache"),
			tierKey: "virtual",
		}

		raw := []byte(`{"id":"lot-4","symbol":"XRP/USD","tier_key":"virtual","opened_at":"2026-11-03T15:07:12.627877Z","closed_at":"2026-11-03T16:00:00Z","entry_order":{"cl_ord_id":"x","ordertype":"market","type":"buy","volume":"11.73"}}`)

		got := cache.Restore(raw)

		So(got, ShouldNotBeNil)
		So(got.ClosedAt, ShouldNotBeNil)
		So(got.ClosedAt.UTC().Format(time.RFC3339), ShouldEqual, "2026-11-03T16:00:00Z")
	})
}

func TestRestoreKeepsFlatLotClosed(t *testing.T) {
	Convey("Open-prefix Restore keeps ClosedAt when fills already flattened the lot", t, func() {
		cache := &Cache{
			System:  runtime.NewSystem(context.Background(), "cache"),
			tierKey: "virtual",
		}

		raw := []byte(`{
			"id":"lot-flat",
			"symbol":"INIT/USD",
			"tier_key":"virtual",
			"opened_at":"2026-11-05T09:56:31Z",
			"closed_at":"2026-11-05T13:19:50Z",
			"fills":[
				{"exec_id":"b1","side":"buy","last_qty":"10","last_price":"1","exec_type":"trade"},
				{"exec_id":"s1","side":"sell","last_qty":"10","last_price":"1.1","exec_type":"trade"},
				{"exec_id":"s2","side":"sell","last_qty":"10","last_price":"0.9","exec_type":"trade"}
			]
		}`)

		got := cache.Restore(raw)

		So(got, ShouldNotBeNil)
		So(got.Flat(), ShouldBeTrue)
		So(got.ClosedAt, ShouldNotBeNil)
		So(got.ExitVWAP.String(), ShouldEqual, "1.1")
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

