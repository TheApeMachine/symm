package manifold

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	mkernel "github.com/theapemachine/nomagique/physics/manifold"
)

func TestUniverseRegisterSymbols(t *testing.T) {
	convey.Convey("Given spot symbols", t, func() {
		viper.Set("signals.manifold.tick_size", 0.01)
		viper.Set("signals.manifold.grid_half_width", 16)
		viper.Set("market.book_depth_levels", 10)

		universe, err := NewUniverse(mkernel.Config{
			GridX: 32,
			GridY: 3,
			GridZ: 16,
		})

		convey.Convey("It should register spot and perpetual lanes for USD pairs", func() {
			convey.So(err, convey.ShouldBeNil)

			universe.registerSymbols([]string{"XBT/USD", "ETH/USD"})

			spot := universe.loadIdentity(InstrumentIdentity{
				Symbol: "XBT/USD",
				Base:   "XBT",
				Lane:   InstrumentLaneSpot,
			})
			perp := universe.loadIdentity(InstrumentIdentity{
				Symbol: "PI_XBTUSD",
				Base:   "XBT",
				Lane:   InstrumentLanePerpetual,
			})

			convey.So(spot, convey.ShouldNotBeNil)
			convey.So(perp, convey.ShouldNotBeNil)
		})
	})
}

func TestUniverseWhaleQtyThreshold(t *testing.T) {
	convey.Convey("Given recent trade flow and book depth", t, func() {
		state := &UniverseState{
			bookDepth: 10,
			bookReady: true,
			midPrice:  50000,
			book: BookUpdate{
				Bids: []BookLevel{{Price: 49999, Qty: 1}, {Price: 49998, Qty: 2}},
				Asks: []BookLevel{{Price: 50001, Qty: 1.5}, {Price: 50002, Qty: 2.5}},
			},
		}
		state.SetTradeQtys([]float64{0.1, 0.2, 0.15, 0.12, 0.18})
		state.SetReturns([]float64{0.01, -0.008, 0.012})

		convey.Convey("It should classify only flow multiples above the dynamic surge threshold as whales", func() {
			threshold := state.whaleQtyThreshold()

			convey.So(math.IsInf(threshold, 1), convey.ShouldBeFalse)
			convey.So(0.5 >= threshold, convey.ShouldBeFalse)
			convey.So(50.0 >= threshold, convey.ShouldBeTrue)
		})
	})
}

func TestMedianAbsoluteUsesMedianOfAbsoluteValues(t *testing.T) {
	convey.Convey("Given signed manifold returns", t, func() {
		values := []float64{-100, -2, 4, 6}

		convey.Convey("It should use the median of absolute values", func() {
			convey.So(medianAbsolute(values), convey.ShouldEqual, 5)
		})
	})
}

func TestUniverseCoordsLanes(t *testing.T) {
	convey.Convey("Given registered spot and perpetual lanes", t, func() {
		viper.Set("signals.manifold.tick_size", 0.01)
		viper.Set("signals.manifold.grid_half_width", 16)
		viper.Set("market.book_depth_levels", 10)

		universe, err := NewUniverse(mkernel.Config{
			GridX: 32,
			GridY: 3,
			GridZ: 16,
		})

		convey.Convey("It should place each instrument lane on a distinct Y cell", func() {
			convey.So(err, convey.ShouldBeNil)

			spot := universe.loadIdentity(InstrumentIdentity{
				Symbol: "XBT/USD",
				Base:   "XBT",
				Lane:   InstrumentLaneSpot,
			})
			perp := universe.loadIdentity(InstrumentIdentity{
				Symbol: "PI_XBTUSD",
				Base:   "XBT",
				Lane:   InstrumentLanePerpetual,
			})
			dated := universe.loadIdentity(InstrumentIdentity{
				Symbol: "FI_XBTUSD_210625",
				Base:   "XBT",
				Lane:   InstrumentLaneDatedFuture,
			})

			m := map[string]uint32{"XBT": 2}
			universe.ranks.Store(&m)

			spotCoords := universe.coords(spot, 0)
			perpCoords := universe.coords(perp, 0)
			datedCoords := universe.coords(dated, 0)

			convey.So(spotCoords.cellY, convey.ShouldEqual, uint32(0))
			convey.So(perpCoords.cellY, convey.ShouldEqual, uint32(1))
			convey.So(datedCoords.cellY, convey.ShouldEqual, uint32(2))
			convey.So(spotCoords.cellZ, convey.ShouldEqual, uint32(2))
			convey.So(perpCoords.cellZ, convey.ShouldEqual, uint32(2))
		})
	})
}

func TestUniverseRankSpread(t *testing.T) {
	convey.Convey("Given more spot bases than grid Z", t, func() {
		viper.Set("signals.manifold.tick_size", 0.01)
		viper.Set("signals.manifold.grid_half_width", 16)
		viper.Set("market.book_depth_levels", 10)

		universe, err := NewUniverse(mkernel.Config{
			GridX: 32,
			GridY: 3,
			GridZ: 16,
		})

		convey.Convey("It should spread ranks across every Z slice", func() {
			convey.So(err, convey.ShouldBeNil)

			for index := 0; index < 648; index++ {
				base := fmt.Sprintf("SYM%d", index)

				universe.loadIdentity(InstrumentIdentity{
					Symbol: fmt.Sprintf("%s/USD", base),
					Base:   base,
					Lane:   InstrumentLaneSpot,
				})
			}

			universe.recomputeRanks()

			used := make(map[uint32]struct{})

			ranksPtr := universe.ranks.Load()
			if ranksPtr != nil {
				for _, rank := range *ranksPtr {
					used[rank] = struct{}{}
				}
			}

			convey.So(len(used), convey.ShouldEqual, 16)
		})
	})
}

func TestUniverseRanksRecomputeWhenDirty(t *testing.T) {
	convey.Convey("Given price returns that dirty cross-section rank state", t, func() {
		viper.Set("signals.manifold.tick_size", 0.01)
		viper.Set("signals.manifold.grid_half_width", 16)
		viper.Set("market.book_depth_levels", 10)

		field, err := NewField()

		convey.Convey("It should defer recompute until the integration boundary", func() {
			convey.So(err, convey.ShouldBeNil)

			state := field.universe.loadSymbol("XBT/USD")
			convey.So(state, convey.ShouldNotBeNil)

			at := time.Unix(1, 0).UTC()
			field.recordPrice(state, 100, at)
			field.recordPrice(state, 101, at.Add(time.Second))

			convey.So(field.universe.rankDirty.Load(), convey.ShouldBeTrue)
			convey.So(field.universe.rankVersion, convey.ShouldEqual, uint64(0))

			field.universe.recomputeRanksIfDirty()
			version := field.universe.rankVersion

			convey.So(field.universe.rankDirty.Load(), convey.ShouldBeFalse)
			convey.So(version, convey.ShouldBeGreaterThan, uint64(0))

			field.universe.recomputeRanksIfDirty()
			convey.So(field.universe.rankVersion, convey.ShouldEqual, version)
		})
	})
}

func TestUniverseRanksConcurrent(t *testing.T) {
	convey.Convey("Given concurrent rank updates and coordinate lookups", t, func() {
		viper.Set("signals.manifold.tick_size", 0.01)
		viper.Set("signals.manifold.grid_half_width", 16)
		viper.Set("market.book_depth_levels", 10)

		universe, err := NewUniverse(mkernel.Config{
			GridX: 32,
			GridY: 3,
			GridZ: 16,
		})

		convey.Convey("It should not race on the rank map", func() {
			convey.So(err, convey.ShouldBeNil)

			universe.registerSymbols([]string{"XBT/USD", "ETH/USD", "SOL/USD"})
			state := universe.loadSymbol("XBT/USD")

			convey.So(state, convey.ShouldNotBeNil)

			done := make(chan struct{})

			go func() {
				defer close(done)

				for index := 0; index < 200; index++ {
					state.AppendReturn(0.001*float64(index%5), 64)
					universe.recomputeRanks()
				}
			}()

			for index := 0; index < 200; index++ {
				_ = universe.coords(state, float64(index%7))
			}

			<-done
		})
	})
}

func TestUniverseConfigureTickFromBookFallback(t *testing.T) {
	convey.Convey("Given a single-level book snapshot", t, func() {
		viper.Set("signals.manifold.tick_size", 0.00000001)
		viper.Set("signals.manifold.grid_half_width", 16)
		viper.Set("market.book_depth_levels", 10)

		universe, err := NewUniverse(mkernel.Config{
			GridX: 32,
			GridY: 3,
			GridZ: 16,
		})

		convey.Convey("It should keep the configured fallback tick size", func() {
			convey.So(err, convey.ShouldBeNil)

			state := universe.loadSymbol("SHIB/USD")
			configureErr := state.configureTickFromBook(
				[]BookLevel{{Price: 0.00001, Qty: 1}},
				[]BookLevel{{Price: 0.00002, Qty: 1}},
				universe.tickSizeFallback(),
			)

			convey.So(configureErr, convey.ShouldBeNil)
			convey.So(state.tickSize, convey.ShouldAlmostEqual, 0.00000001, 1e-12)
		})
	})
}

func BenchmarkUniverseRecomputeRanks(b *testing.B) {
	viper.Set("signals.manifold.tick_size", 0.01)
	viper.Set("signals.manifold.grid_half_width", 16)
	viper.Set("market.book_depth_levels", 10)

	universe, err := NewUniverse(mkernel.Config{
		GridX: 32,
		GridY: 3,
		GridZ: 16,
	})

	if err != nil {
		b.Fatal(err)
	}

	for index := 0; index < 648; index++ {
		base := fmt.Sprintf("SYM%d", index)

		universe.loadIdentity(InstrumentIdentity{
			Symbol: fmt.Sprintf("%s/USD", base),
			Base:   base,
			Lane:   InstrumentLaneSpot,
		})
	}

	universe.recomputeRanks()

	b.ResetTimer()

	for index := 0; index < b.N; index++ {
		universe.recomputeRanks()
	}
}
