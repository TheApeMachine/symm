package store

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
regionPins names one (source, metric stem) pair per region. The fixture is
verified against PinRegion before use, so a remapping cannot silently move a
fixture metric into another region.
*/
var regionPins = [13]struct {
	source string
	stem   string
}{
	1:  {"hawkes", "intensity:buy"},
	2:  {"hawkes", "branching_ratio"},
	3:  {"hawkes", "intensity:sell"},
	4:  {"cvd", "quantity:buy"},
	5:  {"cvd", "churn"},
	6:  {"cvd", "quantity:sell"},
	7:  {"depthflow", "removed:ask"},
	8:  {"depthflow", "spread"},
	9:  {"depthflow", "removed:bid"},
	10: {"sentiment", "advance_fraction"},
	11: {"morphology", "wasserstein"},
	12: {"sentiment", "decline_fraction"},
}

/*
regionEpoch gives every regionFrame its own epoch, so each fixture's metric
streams start cold instead of inheriting moments from an earlier fixture.
*/
var regionEpoch atomic.Int64

/*
regionFrame builds a finalized frame whose peers hold sizes[region] metrics
pinned to each region, and returns the frame with every metric entry so a
test can assign standardized deformations directly. When warm is true each
metric stream first observes two differing values through the real
finalization path, so the returned metrics have a defined z-score; when warm
is false every returned metric is its stream's first observation.
*/
func regionFrame(grid *Grid, sizes [13]int, warm bool) (*data.Measurement, [13][]*data.MetricEntry) {
	var entries [13][]*data.MetricEntry
	epoch := regionEpoch.Add(1)
	at := time.Unix(1_700_000_000, 0).UTC()
	frame := data.NewMeasurement(epoch, "BTC/USD", "frame", 1, 1)
	frame.At, frame.From = at, at

	for region := 1; region <= 12; region++ {
		if sizes[region] == 0 {
			continue
		}

		pin := regionPins[region]
		history := []float64{1}

		if warm {
			history = []float64{0, 2, 1}
		}

		var peer *data.Measurement

		for _, raw := range history {
			peer = data.NewMeasurement(epoch, "BTC/USD", pin.source, 1, 1)
			peer.At, peer.From = at, at
			metrics := make([]*data.Metric, 0, sizes[region])

			for idx := range sizes[region] {
				label := fmt.Sprintf("%s_%d", pin.stem, idx)
				So(grid.PinRegion(pin.source, label), ShouldEqual, uint8(region))
				metrics = append(metrics, data.NewMetric(label, raw, data.UnitDimensionless, data.TimescaleTick))
			}

			peer.Write(metrics...)
		}

		for entry := range peer.Read() {
			So(entry.Metric, ShouldNotBeNil)
			So(entry.Metric.Standardizable(), ShouldEqual, warm)
			entries[region] = append(entries[region], entry)
		}

		So(entries[region], ShouldHaveLength, sizes[region])
		frame.Peers(peer)
	}

	frame.Write()

	return frame, entries
}

func TestGrid_RegionScores(t *testing.T) {
	Convey("Given a grid", t, func() {
		grid := NewGrid()

		Convey("When one region's deformations are known exactly", func() {
			frame, entries := regionFrame(grid, [13]int{4: 4}, true)

			for idx, value := range []float64{1, -2, 0, 1} {
				entries[4][idx].Metric.Standardized = value
			}

			regions := grid.RegionScores(frame)

			Convey("Its brightness is the standardized half-normal excess of sum|z|", func() {
				want := (4 - 4*math.Sqrt(2/math.Pi)) / math.Sqrt(4*(1-2/math.Pi))

				So(regions.Counts[4], ShouldEqual, 4)
				So(regions.Brightness[4], ShouldAlmostEqual, want, 1e-12)
				So(regions.Winner, ShouldEqual, uint8(4))
				So(regions.RunnerUp, ShouldEqual, noEvidence)
			})
		})

		Convey("When the only evidenced region is quieter than the null", func() {
			frame, entries := regionFrame(grid, [13]int{11: 3}, true)

			for _, entry := range entries[11] {
				entry.Metric.Standardized = 0
			}

			regions := grid.RegionScores(frame)

			Convey("It still wins with negative brightness rather than yielding to a default region", func() {
				So(regions.Brightness[11], ShouldBeLessThan, 0)
				So(regions.Winner, ShouldEqual, uint8(11))
				So(string(grid.Observe(frame)), ShouldEqual, "R11")
			})
		})

		Convey("When every region's deformations are drawn from the null with very unequal metric counts", func() {
			sizes := [13]int{0, 1, 2, 3, 5, 8, 13, 21, 34, 44, 9, 4, 6}
			frame, entries := regionFrame(grid, sizes, true)
			source := rand.New(rand.NewPCG(20261009, 1))
			trials := 20000

			var wins [13]int
			var uncorrectedWins [13]int

			for range trials {
				for region := 1; region <= 12; region++ {
					for _, entry := range entries[region] {
						entry.Metric.Standardized = source.NormFloat64()
					}
				}

				regions := grid.RegionScores(frame)
				wins[regions.Winner]++

				// The pre-fix statistic sum|z|/sqrt(N), recovered exactly from
				// the same draws, shows the test can detect the size bias.
				uncorrected, best := 0.0, uint8(0)

				for region := uint8(1); region <= 12; region++ {
					count := float64(regions.Counts[region])
					sum := regions.Brightness[region]*math.Sqrt(count*halfNormalVariance) + count*halfNormalMean

					if best == 0 || sum/math.Sqrt(count) > uncorrected {
						uncorrected, best = sum/math.Sqrt(count), region
					}
				}

				uncorrectedWins[best]++
			}

			t.Logf("null argmax wins by region R01..R12 (sizes %v): corrected %v, uncorrected %v",
				sizes[1:], wins[1:], uncorrectedWins[1:])

			Convey("Each region wins the argmax at roughly the uniform rate 1/12", func() {
				// Tolerance: +/-25% of 1/12. The half-normal standardization is a
				// CLT correction; for N=1 its right skew leaves a measured ~+9%
				// excess, and binomial noise at 20000 trials is ~2.3% relative.
				uniform := float64(trials) / 12

				for region := 1; region <= 12; region++ {
					So(float64(wins[region]), ShouldBeBetween, 0.75*uniform, 1.25*uniform)
				}

				So(wins[noEvidence], ShouldEqual, 0)
			})

			Convey("Whereas the uncorrected statistic hands most frames to the largest region", func() {
				So(float64(uncorrectedWins[9]), ShouldBeGreaterThan, 0.5*float64(trials))
			})
		})
	})
}

func TestGrid_Observe(t *testing.T) {
	Convey("Given a grid", t, func() {
		grid := NewGrid()

		Convey("When the frame and its peers carry no metric at all", func() {
			frame, _ := regionFrame(grid, [13]int{}, true)
			regions := grid.RegionScores(frame)

			Convey("No region holds evidence and the explicit no-evidence token R00 is emitted", func() {
				So(regions.Counts, ShouldResemble, [13]int{})
				So(regions.Winner, ShouldEqual, noEvidence)
				So(regions.RunnerUp, ShouldEqual, noEvidence)
				So(string(grid.Observe(frame)), ShouldEqual, "R00")
			})
		})

		Convey("When a region's metrics have no defined z-score yet", func() {
			frame, entries := regionFrame(grid, [13]int{5: 4, 8: 2}, false)

			for _, entry := range entries[5] {
				entry.Metric.Standardized = 9
			}

			regions := grid.RegionScores(frame)

			Convey("They are excluded from N and counted as undefined, not scored as quiet evidence", func() {
				So(regions.Counts, ShouldResemble, [13]int{})
				So(regions.Undefined, ShouldEqual, 6)
				So(regions.Winner, ShouldEqual, noEvidence)
				So(string(grid.Observe(frame)), ShouldEqual, "R00")
			})
		})

		Convey("When the frame is nil", func() {
			Convey("The no-evidence token R00 is emitted", func() {
				So(string(grid.Observe(nil)), ShouldEqual, "R00")
			})
		})

		Convey("When several regions hold evidence", func() {
			frame, entries := regionFrame(grid, [13]int{2: 3, 7: 2, 10: 5}, true)

			for region, value := range map[int]float64{2: 0.5, 7: 3, 10: 1.5} {
				for _, entry := range entries[region] {
					entry.Metric.Standardized = value
				}
			}

			regions := grid.RegionScores(frame)

			Convey("Observe emits the RegionScores winner, and the runner-up is the second brightest", func() {
				So(string(grid.Observe(frame)), ShouldEqual, fmt.Sprintf("R%02d", regions.Winner))
				So(regions.Winner, ShouldEqual, uint8(7))
				So(regions.RunnerUp, ShouldEqual, uint8(10))
			})
		})
	})
}

func TestGrid_Unpinned(t *testing.T) {
	Convey("Given solver and unknown sources", t, func() {
		grid := NewGrid()

		Convey("PinRegion leaves them unpinned", func() {
			for _, source := range []string{"resonance", "manifold", "runtime:join", "unknown"} {
				So(grid.PinRegion(source, "surprise"), ShouldEqual, unpinned)
			}
		})

		at := time.Unix(1_700_000_000, 0).UTC()
		solverPeer := func(source string) (*data.Measurement, []*data.MetricEntry) {
			peer := data.NewMeasurement(1, "BTC/USD", source, 1, 1)
			peer.At, peer.From = at, at
			peer.Write(
				data.NewMetric("surprise", 1, data.UnitDimensionless, data.TimescaleTick),
				data.NewMetric("curvature", 1, data.UnitDimensionless, data.TimescaleTick),
			)

			entries := make([]*data.MetricEntry, 0, 2)

			for entry := range peer.Read() {
				entries = append(entries, entry)
			}

			return peer, entries
		}

		Convey("A frame holding only solver metrics has no evidence and counts them", func() {
			frame := data.NewMeasurement(1, "BTC/USD", "frame", 1, 1)
			frame.At, frame.From = at, at

			for _, source := range []string{"resonance", "manifold"} {
				peer, entries := solverPeer(source)

				for _, entry := range entries {
					entry.Metric.Standardized = 9
				}

				frame.Peers(peer)
			}

			frame.Write()

			regions := grid.RegionScores(frame)
			So(regions.Unpinned, ShouldEqual, 4)
			So(regions.Winner, ShouldEqual, noEvidence)
			So(string(grid.Observe(frame)), ShouldEqual, "R00")
		})

		Convey("Solver deformation cannot move the winner chosen by sensory evidence", func() {
			var sizes [13]int
			sizes[1], sizes[2] = 3, 3
			frame, entries := regionFrame(grid, sizes, true)

			for _, entry := range entries[1] {
				entry.Metric.Standardized = 2
			}

			peer, solver := solverPeer("resonance")

			for _, entry := range solver {
				entry.Metric.Standardized = 50
			}

			frame.Peers(peer)

			regions := grid.RegionScores(frame)
			So(regions.Unpinned, ShouldEqual, 2)
			So(regions.Counts[2], ShouldEqual, 3)
			So(regions.Winner, ShouldEqual, uint8(1))
		})
	})
}
