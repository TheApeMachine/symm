package manifold

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	mkernel "github.com/theapemachine/nomagique/physics/manifold"
)

func TestUniverseCoords(t *testing.T) {
	convey.Convey("Given a manifold universe", t, func() {
		config := mkernel.Config{
			GridX:   32,
			GridY:   3,
			GridZ:   16,
			DomainX: 0.64,
			DomainY: 3,
			DomainZ: 16,
		}

		viper.Set("signals.manifold.tick_size", 0.01)
		viper.Set("signals.manifold.grid_half_width", 16)
		viper.Set("market.book_depth_levels", 10)

		universe, err := newUniverse(config)

		convey.Convey("It should wrap price offsets on X, instrument lane on Y, and rank on Z", func() {
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

			universe.ranks["XBT"] = 3

			spotCoords := universe.coords(spot, 4)
			perpCoords := universe.coords(perp, 4)

			convey.So(spotCoords.cellX, convey.ShouldEqual, uint32(20))
			convey.So(spotCoords.cellY, convey.ShouldEqual, uint32(0))
			convey.So(spotCoords.cellZ, convey.ShouldEqual, uint32(3))
			convey.So(perpCoords.cellY, convey.ShouldEqual, uint32(1))
			convey.So(perpCoords.cellZ, convey.ShouldEqual, uint32(3))
		})
	})
}

func TestSignalClassify(t *testing.T) {
	convey.Convey("Given manifold readings", t, func() {
		signal := NewSignal("BTC/USD", logic.NewEntity(logic.EntityBook), nil)

		convey.Convey("It should classify high coherence as systemic herd", func() {
			category, _, _, _, _ := signal.classify(mkernel.Reading{
				CoherenceMag2:  2,
				GuidanceSpeed:  0.1,
				ViscosityProxy: 10,
			})

			convey.So(category, convey.ShouldEqual, logic.CategorySystemicHerd)
		})
	})
}
