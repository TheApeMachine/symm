package manifold

import (
	"fmt"
	"math"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	mkernel "github.com/theapemachine/nomagique/physics/manifold"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

func TestUniverseRegisterSymbols(t *testing.T) {
	convey.Convey("Given spot symbols", t, func() {
		viper.Set("signals.manifold.tick_size", 0.01)
		viper.Set("signals.manifold.grid_half_width", 16)
		viper.Set("market.book_depth_levels", 10)

		universe, err := newUniverse(mkernel.Config{
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
		})
	})
}

func TestUniverseWhaleQtyThreshold(t *testing.T) {
	convey.Convey("Given recent trade flow and book depth", t, func() {
		state := &UniverseState{
			bookDepth: 10,
			bookReady: true,
			midPrice:  50000,
			book: krakenmarket.BookUpdate{
				Bids: []krakenmarket.BookLevel{{Price: 49999, Qty: 1}, {Price: 49998, Qty: 2}},
				Asks: []krakenmarket.BookLevel{{Price: 50001, Qty: 1.5}, {Price: 50002, Qty: 2.5}},
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

func TestUniverseCoordsLanes(t *testing.T) {
	convey.Convey("Given registered spot and perpetual lanes", t, func() {
		viper.Set("signals.manifold.tick_size", 0.01)
		viper.Set("signals.manifold.grid_half_width", 16)
		viper.Set("market.book_depth_levels", 10)

		universe, err := newUniverse(mkernel.Config{
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

		universe, err := newUniverse(mkernel.Config{
			GridX: 32,
			GridY: 3,
			GridZ: 16,
		})

		convey.Convey("It should spread ranks across every Z slice", func() {
			convey.So(err, convey.ShouldBeNil)

			for index := 0; index < 648; index++ {
				base := fmt.Sprintf("SYM%d", index)

				universe.loadIdentity(krakenmarket.InstrumentIdentity{
					Symbol: fmt.Sprintf("%s/USD", base),
					Base:   base,
					Lane:   krakenmarket.InstrumentLaneSpot,
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

func TestUniverseRanksConcurrent(t *testing.T) {
	convey.Convey("Given concurrent rank updates and coordinate lookups", t, func() {
		viper.Set("signals.manifold.tick_size", 0.01)
		viper.Set("signals.manifold.grid_half_width", 16)
		viper.Set("market.book_depth_levels", 10)

		universe, err := newUniverse(mkernel.Config{
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

		universe, err := newUniverse(mkernel.Config{
			GridX: 32,
			GridY: 3,
			GridZ: 16,
		})

		convey.Convey("It should keep the configured fallback tick size", func() {
			convey.So(err, convey.ShouldBeNil)

			state := universe.loadSymbol("SHIB/USD")
			configureErr := state.configureTickFromBook(
				[]krakenmarket.BookLevel{{Price: 0.00001, Qty: 1}},
				[]krakenmarket.BookLevel{{Price: 0.00002, Qty: 1}},
				universe.tickSizeFallback(),
			)

			convey.So(configureErr, convey.ShouldBeNil)
			convey.So(state.tickSize, convey.ShouldEqual, 0.00000001)
		})
	})
}

func BenchmarkUniverseRecomputeRanks(b *testing.B) {
	viper.Set("signals.manifold.tick_size", 0.01)
	viper.Set("signals.manifold.grid_half_width", 16)
	viper.Set("market.book_depth_levels", 10)

	universe, err := newUniverse(mkernel.Config{
		GridX: 32,
		GridY: 3,
		GridZ: 16,
	})

	if err != nil {
		b.Fatal(err)
	}

	for index := 0; index < 648; index++ {
		base := fmt.Sprintf("SYM%d", index)

		universe.loadIdentity(krakenmarket.InstrumentIdentity{
			Symbol: fmt.Sprintf("%s/USD", base),
			Base:   base,
			Lane:   krakenmarket.InstrumentLaneSpot,
		})
	}

	universe.recomputeRanks()

	b.ResetTimer()

	for index := 0; index < b.N; index++ {
		universe.recomputeRanks()
	}
}
