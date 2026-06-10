package manifold

import (
	"math"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric/physics"
)

func TestUniverseRegisterSymbols(t *testing.T) {
	convey.Convey("Given spot symbols", t, func() {
		viper.Set("signals.manifold.tick_size", 0.01)
		viper.Set("signals.manifold.grid_half_width", 16)
		viper.Set("market.book_depth_levels", 10)

		universe, err := newUniverse(physics.Config{
			GridX: 32,
			GridY: 3,
			GridZ: 16,
		})

		convey.Convey("It should register spot and perpetual lanes for USD pairs", func() {
			convey.So(err, convey.ShouldBeNil)

			universe.registerSymbols([]string{"XBT/USD", "ETH/USD"})

			spot := universe.loadIdentity(krakenmarket.InstrumentIdentity{
				Symbol: "XBT/USD",
				Base:   "XBT",
				Lane:   krakenmarket.InstrumentLaneSpot,
			})
			perp := universe.loadIdentity(krakenmarket.InstrumentIdentity{
				Symbol: "PI_XBTUSD",
				Base:   "XBT",
				Lane:   krakenmarket.InstrumentLanePerpetual,
			})

			convey.So(spot, convey.ShouldNotBeNil)
			convey.So(perp, convey.ShouldNotBeNil)
			convey.So(spotSymbolForBase(universe, "XBT"), convey.ShouldEqual, "XBT/USD")
		})
	})
}

func TestUniverseWhaleQtyThreshold(t *testing.T) {
	convey.Convey("Given recent trade flow and book depth", t, func() {
		state := &UniverseState{
			bookDepth: 10,
			tradeQtys: []float64{0.1, 0.2, 0.15, 0.12, 0.18},
			returns:   []float64{0.01, -0.008, 0.012},
			bookReady: true,
			midPrice:  50000,
			book: krakenmarket.BookUpdate{
				Bids: []krakenmarket.BookLevel{{Price: 49999, Qty: 1}, {Price: 49998, Qty: 2}},
				Asks: []krakenmarket.BookLevel{{Price: 50001, Qty: 1.5}, {Price: 50002, Qty: 2.5}},
			},
		}

		convey.Convey("It should classify only flow multiples above the dynamic surge threshold as whales", func() {
			threshold := state.whaleQtyThreshold()

			convey.So(math.IsInf(threshold, 1), convey.ShouldBeFalse)
			convey.So(0.5 >= threshold, convey.ShouldBeFalse)
			convey.So(50.0 >= threshold, convey.ShouldBeTrue)
		})
	})
}

func TestUniverseCoordsLanes(t *testing.T) {
	convey.Convey("Given registered spot and perpetual lanes", t, func() {
		viper.Set("signals.manifold.tick_size", 0.01)
		viper.Set("signals.manifold.grid_half_width", 16)
		viper.Set("market.book_depth_levels", 10)

		universe, err := newUniverse(physics.Config{
			GridX: 32,
			GridY: 3,
			GridZ: 16,
		})

		convey.Convey("It should place each instrument lane on a distinct Y cell", func() {
			convey.So(err, convey.ShouldBeNil)

			spot := universe.loadIdentity(krakenmarket.InstrumentIdentity{
				Symbol: "XBT/USD",
				Base:   "XBT",
				Lane:   krakenmarket.InstrumentLaneSpot,
			})
			perp := universe.loadIdentity(krakenmarket.InstrumentIdentity{
				Symbol: "PI_XBTUSD",
				Base:   "XBT",
				Lane:   krakenmarket.InstrumentLanePerpetual,
			})
			dated := universe.loadIdentity(krakenmarket.InstrumentIdentity{
				Symbol: "FI_XBTUSD_210625",
				Base:   "XBT",
				Lane:   krakenmarket.InstrumentLaneDatedFuture,
			})

			universe.rankMu.Lock()
			universe.ranks["XBT"] = 2
			universe.rankMu.Unlock()

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

func TestUniverseRanksConcurrent(t *testing.T) {
	convey.Convey("Given concurrent rank updates and coordinate lookups", t, func() {
		viper.Set("signals.manifold.tick_size", 0.01)
		viper.Set("signals.manifold.grid_half_width", 16)
		viper.Set("market.book_depth_levels", 10)

		universe, err := newUniverse(physics.Config{
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
					state.returns = append(state.returns, 0.001*float64(index%5))
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
