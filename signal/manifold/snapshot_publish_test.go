package manifold

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
)

func TestSignalFieldSnapshot(t *testing.T) {
	Convey("Given an integrated manifold signal field", t, func() {
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

		signal := NewSignal(ctx, pool, dmt.NewTree(""))

		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		signal.field.RegisterSymbols([]string{"XBT/USD"})
		state := signal.field.universe.loadSymbol("XBT/USD")
		state.midPrice = 50000
		state.bookReady = true
		state.book = BookUpdate{
			Symbol: "XBT/USD",
			Bids:   []BookLevel{{Price: 49990, Qty: 1}},
			Asks:   []BookLevel{{Price: 50010, Qty: 1}},
		}
		state.SetTradeQtys([]float64{0.1, 0.2, 0.15})
		state.SetReturns([]float64{0.01, -0.008, 0.012})

		at := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		stepped, integrateErr := signal.field.integrate(at)

		So(integrateErr, ShouldBeNil)
		So(stepped, ShouldBeTrue)

		Convey("It should build a manifold field snapshot payload", func() {
			payload, err := signal.FieldSnapshot(at)

			So(err, ShouldBeNil)
			So(payload["type"], ShouldEqual, "manifold")

			rho, ok := payload["rho"].([][]float64)

			if !ok {
				rawRho, rawOk := payload["rho"].([]any)

				So(rawOk, ShouldBeTrue)
				So(len(rawRho), ShouldBeGreaterThan, 0)
			} else {
				So(len(rho), ShouldBeGreaterThan, 0)
			}

			reading, readingOK := payload["reading"].(map[string]any)

			So(readingOK, ShouldBeTrue)
			So(reading["coherence_mag2"], ShouldNotBeNil)
			So(reading["coherence_mag2"], ShouldBeGreaterThan, 0)
		})

		Convey("It should not build a snapshot before the field has integrated", func() {
			fresh := NewSignal(ctx, nil, dmt.NewTree(""))

			So(fresh, ShouldNotBeNil)

			defer func() {
				_ = fresh.Close()
			}()

			payload, err := fresh.FieldSnapshot(at)

			So(err, ShouldBeNil)
			So(payload, ShouldBeNil)
		})
	})
}

func BenchmarkSignalFieldSnapshot(b *testing.B) {
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

	signal := NewSignal(ctx, pool, dmt.NewTree(""))

	if signal == nil {
		b.Fatal("signal is nil")
	}

	defer func() {
		_ = signal.Close()
	}()

	signal.field.RegisterSymbols([]string{"XBT/USD"})
	state := signal.field.universe.loadSymbol("XBT/USD")
	state.midPrice = 50000
	state.bookReady = true
	state.book = BookUpdate{
		Symbol: "XBT/USD",
		Bids:   []BookLevel{{Price: 49990, Qty: 1}},
		Asks:   []BookLevel{{Price: 50010, Qty: 1}},
	}
	state.SetTradeQtys([]float64{0.1, 0.2, 0.15})
	state.SetReturns([]float64{0.01, -0.008, 0.012})

	at := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	stepped, integrateErr := signal.field.integrate(at)

	if integrateErr != nil || !stepped {
		b.Fatal(integrateErr)
	}

	b.ReportAllocs()

	for b.Loop() {
		if _, err := signal.FieldSnapshot(at); err != nil {
			b.Fatal(err)
		}
	}
}
