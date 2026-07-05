package manifold

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
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
		viper.Set("types.book_depth_levels", 10)

		universe, err := NewUniverse(config)

		convey.Convey("It should wrap price offsets on X, instrument lane on Y, and rank on Z", func() {
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

			rMap := map[string]uint32{"XBT": 3}
			universe.ranks.Store(&rMap)

			spotCoords := universe.coords(spot, 4)
			perpCoords := universe.coords(perp, 4)

			convey.So(spotCoords.cellX, convey.ShouldEqual, uint32(wrapCell(4+spot.halfWidth, 32)))
			convey.So(spotCoords.cellY, convey.ShouldEqual, uint32(0))
			convey.So(spotCoords.cellZ, convey.ShouldEqual, uint32(3))
			convey.So(perpCoords.cellY, convey.ShouldEqual, uint32(1))
			convey.So(perpCoords.cellZ, convey.ShouldEqual, uint32(3))
		})
	})
}
