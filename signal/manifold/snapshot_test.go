package manifold

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	mkernel "github.com/theapemachine/nomagique/physics/manifold")

func TestFieldSnapshotPayload(t *testing.T) {
	convey.Convey("Given an integrated manifold field", t, func() {
		viper.Set("signals.manifold.tick_size", 0.01)
		viper.Set("signals.manifold.grid_half_width", 8)
		viper.Set("signals.manifold.grid_x", 16)
		viper.Set("signals.manifold.grid_y", 1)
		viper.Set("signals.manifold.grid_z", 8)
		viper.Set("signals.manifold.max_modes", 8)
		viper.Set("signals.manifold.integration_interval", "100ms")
		viper.Set("market.book_depth_levels", 4)

		field, err := NewField()

		convey.Convey("It should publish rho projection and reading metadata", func() {
			convey.So(err, convey.ShouldBeNil)

			field.RegisterSymbols([]string{"XBT/USD"})
			state := field.universe.loadSymbol("XBT/USD")
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
			stepped, integrateErr := field.integrate(at)

			convey.So(integrateErr, convey.ShouldBeNil)
			convey.So(stepped, convey.ShouldBeTrue)

			payload, payloadErr := field.snapshotPayload(at)

			convey.So(payloadErr, convey.ShouldBeNil)
			convey.So(payload, convey.ShouldNotBeNil)
			convey.So(payload["type"], convey.ShouldEqual, "manifold")

			rho, ok := payload["rho"].([][]float64)

			convey.So(ok, convey.ShouldBeTrue)
			convey.So(len(rho), convey.ShouldBeGreaterThan, 0)
			convey.So(len(rho[0]), convey.ShouldBeGreaterThan, 0)

			reading, readingOK := payload["reading"].(map[string]any)

			convey.So(readingOK, convey.ShouldBeTrue)
			convey.So(reading["coherence_mag2"], convey.ShouldNotBeNil)
			convey.So(reading["coherence_mag2"], convey.ShouldBeGreaterThan, 0)

			field.Close()
		})
	})
}

func TestFieldSnapshotPayloadRejectsNonFinite(t *testing.T) {
	convey.Convey("Given a manifold reading with NaN", t, func() {
		viper.Set("signals.manifold.tick_size", 0.01)
		viper.Set("signals.manifold.grid_half_width", 8)
		viper.Set("signals.manifold.grid_x", 16)
		viper.Set("signals.manifold.grid_y", 1)
		viper.Set("signals.manifold.grid_z", 8)
		viper.Set("signals.manifold.max_modes", 8)
		viper.Set("signals.manifold.integration_interval", "100ms")
		viper.Set("market.book_depth_levels", 4)

		field, err := NewField()

		convey.Convey("It should refuse to publish a non-json-safe snapshot", func() {
			convey.So(err, convey.ShouldBeNil)

			field.RegisterSymbols([]string{"XBT/USD"})
			state := field.universe.loadSymbol("XBT/USD")
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

			stepped, integrateErr := field.integrate(at)

			convey.So(integrateErr, convey.ShouldBeNil)
			convey.So(stepped, convey.ShouldBeTrue)

			field.lastReading = mkernel.Reading{
				CoherenceMag2: math.NaN(),
			}
			field.lastCarriers = nil

			payload, payloadErr := field.snapshotPayload(at)

			convey.So(payload, convey.ShouldBeNil)
			convey.So(payloadErr, convey.ShouldNotBeNil)

			field.Close()
		})
	})
}

func TestSnapshotPayloadJSONSafe(t *testing.T) {
	convey.Convey("Given an integrated manifold field", t, func() {
		viper.Set("signals.manifold.tick_size", 0.01)
		viper.Set("signals.manifold.grid_half_width", 8)
		viper.Set("signals.manifold.grid_x", 16)
		viper.Set("signals.manifold.grid_y", 1)
		viper.Set("signals.manifold.grid_z", 8)
		viper.Set("signals.manifold.max_modes", 8)
		viper.Set("signals.manifold.integration_interval", "100ms")
		viper.Set("market.book_depth_levels", 4)

		field, err := NewField()

		convey.Convey("It should marshal the snapshot payload to JSON", func() {
			convey.So(err, convey.ShouldBeNil)

			field.RegisterSymbols([]string{"XBT/USD"})
			state := field.universe.loadSymbol("XBT/USD")
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
			stepped, integrateErr := field.integrate(at)

			convey.So(integrateErr, convey.ShouldBeNil)
			convey.So(stepped, convey.ShouldBeTrue)

			payload, payloadErr := field.snapshotPayload(at)

			convey.So(payloadErr, convey.ShouldBeNil)
			convey.So(payload, convey.ShouldNotBeNil)

			_, marshalErr := json.Marshal(payload)

			convey.So(marshalErr, convey.ShouldBeNil)

			field.Close()
		})
	})
}
