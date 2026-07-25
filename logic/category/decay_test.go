package category

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestDecayIdle(t *testing.T) {
	Convey("Given a strengthened edge that goes idle on later cuts", t, func() {
		graph := NewGraph()
		at := time.Unix(10, 0).UTC()
		thesis := types.NewThesis()
		thesis.At = at
		spoof := 0.9
		fill := 0.7
		publishPair := func(stamp time.Time, spoofMass, fillMass float64) []types.Category {
			thesis = types.NewThesis()
			thesis.At = stamp
			thesis.Publish(types.SourceDepthFlow, []*types.Measurement{
				measurementWithMetric(
					types.SourceDepthFlow, "SIM/USD", stamp,
					types.MetricSpoofScore, spoofMass, &spoofMass, 2*time.Second,
				),
			})
			thesis.Publish(types.SourceToxicity, []*types.Measurement{
				measurementWithMetric(
					types.SourceToxicity, "SIM/USD", stamp,
					types.MetricFillVolume, fillMass, &fillMass, 2*time.Second,
				),
			})

			return Compose(thesis, "SIM/USD")
		}

		graph.Update(at, thesis, publishPair(at, spoof, fill))
		first := graph.Weight("SIM/USD", types.SpoofTrap, types.HardSupport, Contradicts)
		So(first, ShouldBeGreaterThan, 0)

		graph.Update(at.Add(time.Second), thesis, publishPair(at.Add(time.Second), spoof, fill))
		strengthened := graph.Weight("SIM/USD", types.SpoofTrap, types.HardSupport, Contradicts)

		Convey("When only an unrelated category stays active later", func() {
			idleAt := at.Add(5 * time.Second)
			ignition := 0.8
			idle := types.NewThesis()
			idle.At = idleAt
			idle.Publish(types.SourcePumpDump, []*types.Measurement{
				measurementWithMetric(
					types.SourcePumpDump, "SIM/USD", idleAt,
					types.MetricIgnition, ignition, &ignition, 2*time.Second,
				),
			})
			graph.Update(idleAt, idle, Compose(idle, "SIM/USD"))
			decayed := graph.Weight("SIM/USD", types.SpoofTrap, types.HardSupport, Contradicts)

			Convey("It decays the idle Contradicts weight by symbol cadence", func() {
				So(strengthened, ShouldBeGreaterThan, first)
				So(decayed, ShouldBeLessThan, strengthened)
				So(decayed, ShouldBeGreaterThan, 0)
			})
		})
	})
}
