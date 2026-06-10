package manifold

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/numeric/physics"
)

func TestSnapshotReading(t *testing.T) {
	convey.Convey("Given carrier oscillators and a zero GPU mode reading", t, func() {
		gpu := physics.Reading{}
		carriers := []fieldCarrier{
			{
				role:   "symbol",
				symbol: "XBT/USD",
				oscillator: physics.Oscillator{
					Amplitude: 0.2,
					VelX:      0.1,
				},
			},
			{
				role:   "symbol",
				symbol: "ETH/USD",
				oscillator: physics.Oscillator{
					Amplitude: 0.4,
					VelZ:      0.3,
				},
			},
		}

		convey.Convey("It should publish cross-section mode statistics on the snapshot", func() {
			reading := snapshotReading(gpu, carriers)

			convey.So(reading.CoherenceMag2, convey.ShouldAlmostEqual, 0.1, 0.0001)
			convey.So(reading.GuidanceSpeed, convey.ShouldAlmostEqual, 0.2, 0.0001)
		})
	})
}
