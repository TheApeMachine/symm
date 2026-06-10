package manifold

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric/physics"
)

func TestFieldSnapshotReadingUsesStepState(t *testing.T) {
	convey.Convey("Given an integrated manifold field", t, func() {
		viper.Set("signals.manifold.tick_size", 0.01)
		viper.Set("signals.manifold.grid_half_width", 8)
		viper.Set("signals.manifold.grid_x", 16)
		viper.Set("signals.manifold.grid_y", 1)
		viper.Set("signals.manifold.grid_z", 8)
		viper.Set("signals.manifold.max_modes", 8)
		viper.Set("signals.manifold.integration_interval", "100ms")
		viper.Set("market.book_depth_levels", 4)

		field, err := newField()

		convey.Convey("It should publish the last GPU step reading without Go recomputation", func() {
			convey.So(err, convey.ShouldBeNil)

			field.RegisterSymbols([]string{"XBT/USD"})
			state := field.universe.loadSymbol("XBT/USD")
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
			field.lastStepAt = at.Add(-time.Second)

			stepped, integrateErr := field.integrate(at)

			convey.So(integrateErr, convey.ShouldBeNil)
			convey.So(stepped, convey.ShouldBeTrue)

			field.lastReading = physics.Reading{
				PressureGradNorm: 0.42,
				CoherenceMag2:    0.05,
			}

			payload, payloadErr := field.snapshotPayload(at)

			convey.So(payloadErr, convey.ShouldBeNil)
			convey.So(payload, convey.ShouldNotBeNil)

			reading, ok := payload["reading"].(map[string]any)

			convey.So(ok, convey.ShouldBeTrue)
			convey.So(reading["pressure_grad_norm"], convey.ShouldEqual, 0.42)
			convey.So(reading["coherence_mag2"], convey.ShouldEqual, 0.05)

			field.Close()
		})
	})
}
