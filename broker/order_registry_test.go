package broker

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/types"
)

func TestOrderRegistryRejectStaleEntries(t *testing.T) {
	Convey("Given a submitted entry past transit ttl", t, func() {
		viper.Set("trading.entry.transit_ttl", time.Millisecond)

		registry := NewOrderRegistry()
		clOrdID := "stale-entry"
		registry.Store(clOrdID, types.KrakenMessage{
			Method: trading.MethodAddOrder,
			Params: &trading.AddParams{
				ClOrdID:       clOrdID,
				EntryQueuedAt: time.Now().Add(-time.Second),
			},
		})

		time.Sleep(2 * time.Millisecond)

		Convey("It should reject stale entries on scan", func() {
			rejected := registry.RejectStaleEntries()

			So(rejected, ShouldContain, clOrdID)
		})
	})
}

func TestOrderRegistryStoreLoad(t *testing.T) {
	Convey("Given one tracked order", t, func() {
		registry := NewOrderRegistry()
		frame := types.KrakenMessage{Method: trading.MethodAddOrder}

		registry.Store("abc", frame)

		Convey("It should round-trip the frame", func() {
			loaded, ok := registry.Load("abc")

			So(ok, ShouldBeTrue)
			So(loaded.Method, ShouldEqual, trading.MethodAddOrder)
		})
	})
}
