package manifold

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

func TestSystemPublishSnapshot(t *testing.T) {
	Convey("Given an integrated manifold system field", t, func() {
		viper.Set("signals.manifold.measurements_capacity", 16)
		viper.Set("signals.manifold.tick_size", 0.01)
		viper.Set("signals.manifold.grid_half_width", 8)
		viper.Set("signals.manifold.grid_x", 16)
		viper.Set("signals.manifold.grid_y", 1)
		viper.Set("signals.manifold.grid_z", 8)
		viper.Set("signals.manifold.max_modes", 8)
		viper.Set("signals.manifold.integration_interval", "100ms")
		viper.Set("market.book_depth_levels", 4)

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 4, nil)
		defer pool.Close()

		system := NewSystem(ctx, pool)

		So(system, ShouldNotBeNil)

		system.field.RegisterSymbols([]string{"XBT/USD"})
		state := system.field.universe.loadSymbol("XBT/USD")
		state.midPrice = 50000
		state.bookReady = true
		state.book = krakenmarket.BookUpdate{
			Symbol: "XBT/USD",
			Bids:   []krakenmarket.BookLevel{{Price: 49990, Qty: 1}},
			Asks:   []krakenmarket.BookLevel{{Price: 50010, Qty: 1}},
		}
		state.tradeQtys = []float64{0.1, 0.2, 0.15}
		state.returns = []float64{0.01, -0.008, 0.012}

		at := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		stepped, integrateErr := system.field.integrate(at)

		So(integrateErr, ShouldBeNil)
		So(stepped, ShouldBeTrue)

		uiBus := internal.NewBus(ctx, pool, nil, []internal.Subscription{internal.Subscribe("ui", "test-ui")})

		Convey("It should publish a manifold field snapshot on the ui bus", func() {
			received := make(chan map[string]any, 1)

			go func() {
				for {
					row, receiveErr := uiBus.Receive("ui")

					if receiveErr != nil {
						return
					}

					if row == nil {
						continue
					}

					frame, ok := row.Value.(map[string]any)

					if !ok || frame["type"] != "manifold" {
						continue
					}

					received <- frame
					return
				}
			}()

			err := system.publishSnapshot(at)

			So(err, ShouldBeNil)

			var frame map[string]any

			select {
			case frame = <-received:
			case <-time.After(2 * time.Second):
				So("ui manifold_snapshot", ShouldEqual, "received")
			}

			So(frame["type"], ShouldEqual, "manifold")

			rho, ok := frame["rho"].([][]float64)

			So(ok, ShouldBeTrue)
			So(len(rho), ShouldBeGreaterThan, 0)

			reading, readingOK := frame["reading"].(map[string]any)

			So(readingOK, ShouldBeTrue)
			So(reading["coherence_mag2"], ShouldNotBeNil)
			So(reading["coherence_mag2"], ShouldBeGreaterThan, 0)
		})

		Convey("It should not publish before the field has integrated", func() {
			fresh := NewSystem(ctx, pool)

			So(fresh, ShouldNotBeNil)

			err := fresh.publishSnapshot(at)

			So(err, ShouldBeNil)
		})
	})
}

func BenchmarkSystemPublishSnapshot(b *testing.B) {
	viper.Set("signals.manifold.measurements_capacity", 16)
	viper.Set("signals.manifold.tick_size", 0.01)
	viper.Set("signals.manifold.grid_half_width", 8)
	viper.Set("signals.manifold.grid_x", 16)
	viper.Set("signals.manifold.grid_y", 1)
	viper.Set("signals.manifold.grid_z", 8)
	viper.Set("signals.manifold.max_modes", 8)
	viper.Set("signals.manifold.integration_interval", "100ms")
	viper.Set("market.book_depth_levels", 4)

	ctx := context.Background()
	pool := qpool.NewQ[any](ctx, 2, 4, nil)
	defer pool.Close()

	system := NewSystem(ctx, pool)

	if system == nil {
		b.Fatal("system is nil")
	}

	system.field.RegisterSymbols([]string{"XBT/USD"})
	state := system.field.universe.loadSymbol("XBT/USD")
	state.midPrice = 50000
	state.bookReady = true
	state.book = krakenmarket.BookUpdate{
		Symbol: "XBT/USD",
		Bids:   []krakenmarket.BookLevel{{Price: 49990, Qty: 1}},
		Asks:   []krakenmarket.BookLevel{{Price: 50010, Qty: 1}},
	}
	state.tradeQtys = []float64{0.1, 0.2, 0.15}
	state.returns = []float64{0.01, -0.008, 0.012}

	at := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	stepped, integrateErr := system.field.integrate(at)

	if integrateErr != nil || !stepped {
		b.Fatal(integrateErr)
	}

	b.ReportAllocs()

	for b.Loop() {
		if err := system.publishSnapshot(at); err != nil {
			b.Fatal(err)
		}
	}
}
