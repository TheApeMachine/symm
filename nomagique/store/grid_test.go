package store

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
regionPins names one (source, metric stem) pair per region (indices 0..11 for regions 1..12).
The fixture is verified against PinRegion before use, so a remapping cannot silently move a
fixture metric into another region.
*/
var regionPins = [12]struct {
	source string
	stem   string
}{
	0:  {"hawkes", "intensity:buy"},
	1:  {"hawkes", "branching_ratio"},
	2:  {"hawkes", "intensity:sell"},
	3:  {"cvd", "quantity:buy"},
	4:  {"cvd", "churn"},
	5:  {"cvd", "quantity:sell"},
	6:  {"depthflow", "removed:ask"},
	7:  {"depthflow", "spread"},
	8:  {"depthflow", "removed:bid"},
	9:  {"sentiment", "advance_fraction"},
	10: {"morphology", "wasserstein"},
	11: {"sentiment", "decline_fraction"},
}

/*
regionEpoch gives every regionFrame its own epoch, so each fixture's metric
streams start cold instead of inheriting moments from an earlier fixture.
*/
var regionEpoch atomic.Int64

/*
regionFrame builds a finalized frame whose peers hold sizes[idx] metrics
pinned to each region, and returns the frame with every metric entry so a
test can assign standardized deformations directly. When warm is true each
metric stream first observes core.MinimumPrior differing values through
the real finalization path, so the returned metrics have a defined z-score; when warm
is false every returned metric is its stream's first observation.
*/
func regionFrame(grid *Grid, sizes [12]int, warm bool) (*data.Measurement, [12][]*data.MetricEntry) {
	var entries [12][]*data.MetricEntry
	epoch := regionEpoch.Add(1)
	at := time.Unix(1_700_000_000, 0).UTC()
	frame := data.NewMeasurement(epoch, "BTC/USD", "frame", 1, 1)
	frame.At, frame.From = at, at

	for idx := range 12 {
		if sizes[idx] == 0 {
			continue
		}

		pin := regionPins[idx]
		region := uint8(idx)
		history := []float64{1}

		if warm {
			history = nil

			for hIdx := range int(core.MinimumPrior) {
				history = append(history, float64(2*(hIdx%2)))
			}

			history = append(history, 1)
		}

		var peer *data.Measurement

		for hIdx, raw := range history {
			values := make(map[string]float64, sizes[idx])
			for sIdx := range sizes[idx] {
				label := fmt.Sprintf("%s_%d", pin.stem, sIdx)
				reg, ok := grid.PinRegion(pin.source, label)
				So(ok, ShouldBeTrue)
				So(reg, ShouldEqual, region)
				values[label] = raw
			}

			if peer == nil {
				peer = data.NewMeasurement(epoch, "BTC/USD", pin.source, int64(hIdx+1), int64(hIdx+1))
				peer.At, peer.From = at, at
				metrics := make([]*data.Metric, 0, len(values))
				for k, v := range values {
					metrics = append(metrics, data.NewMetric(k, v, data.UnitDimensionless, data.TimescaleTick))
				}
				peer.Write(metrics...)
			} else {
				peer = peer.Next(pin.source, values)
				peer.At, peer.From = at, at
			}
		}

		for entry := range peer.Read() {
			So(entry.Metric, ShouldNotBeNil)
			So(entry.Metric.Standardizable(), ShouldEqual, warm)
			entries[idx] = append(entries[idx], entry)
		}

		So(entries[idx], ShouldHaveLength, sizes[idx])
		frame.Peers(peer)
	}

	frame.Write()

	return frame, entries
}

func TestGrid_RegionScores(t *testing.T) {
	Convey("Given a grid", t, func() {
		grid := NewGrid()

		Convey("When one region's deformations are known exactly", func() {
			sizes := [12]int{1, 1, 1, 4, 1, 1, 1, 1, 1, 1, 1, 1}
			frame, entries := regionFrame(grid, sizes, true)

			for idx, value := range []float64{1, -2, 0, 1} {
				entries[3][idx].Metric.Standardized = value
			}

			regions := grid.RegionScores(frame)

			Convey("Its brightness is the standardized half-normal excess of sum|z|", func() {
				want := (4.0 - 4*math.Sqrt(2/math.Pi)) / math.Sqrt(4*(1-2/math.Pi))

				So(regions.Counts[3], ShouldEqual, 4)
				So(regions.Brightness[3], ShouldAlmostEqual, want, 1e-12)
				So(regions.HasEvidence, ShouldBeTrue)
			})
		})

		Convey("When all evidenced regions are quieter than the null", func() {
			sizes := [12]int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 3, 1}
			frame, entries := regionFrame(grid, sizes, true)

			for region := range 12 {
				for _, entry := range entries[region] {
					entry.Metric.Standardized = 0
				}
			}

			regions := grid.RegionScores(frame)

			Convey("They win with negative brightness rather than yielding to a default region", func() {
				for region := range 12 {
					So(regions.Brightness[region], ShouldBeLessThan, 0)
				}
				So(regions.HasEvidence, ShouldBeTrue)
				So(grid.Observe(frame), ShouldNotBeNil)
			})
		})

		Convey("When every region's deformations are drawn from the null with very unequal metric counts", func() {
			sizes := [12]int{1, 2, 3, 5, 8, 13, 21, 34, 44, 9, 4, 6}
			frame, entries := regionFrame(grid, sizes, true)
			source := rand.New(rand.NewPCG(20261009, 1))
			trials := 20000

			var wins [12]int
			var uncorrectedWins [12]int

			for range trials {
				for idx := range 12 {
					for _, entry := range entries[idx] {
						entry.Metric.Standardized = source.NormFloat64()
					}
				}

				regions := grid.RegionScores(frame)
				if regions.HasEvidence {
					wins[regions.Winner]++
				}

				// The pre-fix statistic sum|z|/sqrt(N), recovered exactly from
				// the same draws, shows the test can detect the size bias.
				uncorrected, best := 0.0, uint8(0)

				for idx := range uint8(12) {
					count := float64(regions.Counts[idx])
					sum := regions.Brightness[idx]*math.Sqrt(count*halfNormalVariance) + count*halfNormalMean

					if idx == 0 || sum/math.Sqrt(count) > uncorrected {
						uncorrected, best = sum/math.Sqrt(count), idx
					}
				}

				uncorrectedWins[best]++
			}

			t.Logf("null argmax wins by region R00..R11 (sizes %v): corrected %v, uncorrected %v",
				sizes, wins, uncorrectedWins)

			Convey("Each region wins the argmax at roughly the uniform rate 1/12", func() {
				// Tolerance: +/-25% of 1/12. The half-normal standardization is a
				// CLT correction; for N=1 its right skew leaves a measured ~+9%
				// excess, and binomial noise at 20000 trials is ~2.3% relative.
				uniform := float64(trials) / 12

				for idx := range 12 {
					So(float64(wins[idx]), ShouldBeBetween, 0.75*uniform, 1.25*uniform)
				}
			})

			Convey("Whereas the uncorrected statistic hands most frames to the largest region", func() {
				So(float64(uncorrectedWins[8]), ShouldBeGreaterThan, 0.5*float64(trials))
			})
		})
	})
}

func TestGrid_Observe(t *testing.T) {
	Convey("Given a grid", t, func() {
		grid := NewGrid()

		Convey("When the frame and its peers carry no metric at all", func() {
			frame, _ := regionFrame(grid, [12]int{}, true)
			regions := grid.RegionScores(frame)

			Convey("No region holds evidence and no token is emitted", func() {
				So(regions.Counts, ShouldResemble, [12]int{})
				So(regions.HasEvidence, ShouldBeFalse)
				So(grid.Observe(frame), ShouldBeNil)
			})
		})

		Convey("When a region's metrics have no defined z-score yet", func() {
			frame, entries := regionFrame(grid, [12]int{4: 4, 7: 2}, false)

			for _, entry := range entries[4] {
				entry.Metric.Standardized = 9
			}

			regions := grid.RegionScores(frame)

			Convey("Scoring does not continue on warming frames with no confidence and emits no token", func() {
				So(regions.Counts, ShouldResemble, [12]int{})
				So(regions.HasEvidence, ShouldBeFalse)
				So(grid.Observe(frame), ShouldBeNil)
			})
		})

		Convey("When the frame is nil", func() {
			Convey("No token is emitted", func() {
				So(grid.Observe(nil), ShouldBeNil)
			})
		})

		Convey("When any region is missing evidence", func() {
			frame, _ := regionFrame(grid, [12]int{1: 3, 6: 2, 9: 5}, true)
			regions := grid.RegionScores(frame)

			Convey("Scoring stops and emits no token", func() {
				So(regions.HasEvidence, ShouldBeFalse)
				So(grid.Observe(frame), ShouldBeNil)
			})
		})

		Convey("When all regions hold evidence", func() {
			sizes := [12]int{1, 3, 1, 1, 1, 1, 2, 1, 1, 5, 1, 1}
			frame, entries := regionFrame(grid, sizes, true)

			for idx, value := range map[int]float64{1: 0.5, 6: 3, 9: 1.5} {
				for _, entry := range entries[idx] {
					entry.Metric.Standardized = value
				}
			}

			regions := grid.RegionScores(frame)

			Convey("Observe emits the RegionScores winner, and the runner-up is the second brightest", func() {
				So(string(grid.Observe(frame)), ShouldEqual, fmt.Sprintf("R%02d", regions.Winner))
				So(regions.Winner, ShouldEqual, uint8(6))
				So(regions.RunnerUp, ShouldEqual, uint8(9))
			})
		})
	})
}

func TestGrid_Unpinned(t *testing.T) {
	Convey("Given solver and unknown sources", t, func() {
		grid := NewGrid()

		Convey("PinRegion leaves them unpinned", func() {
			for _, source := range []string{"resonance", "manifold", "runtime:join", "unknown"} {
				_, ok := grid.PinRegion(source, "surprise")
				So(ok, ShouldBeFalse)
			}
		})

		at := time.Unix(1_700_000_000, 0).UTC()
		solverPeer := func(source string) (*data.Measurement, []*data.MetricEntry) {
			peer := data.NewMeasurement(1, "BTC/USD", source, 1, 1)
			peer.At, peer.From = at, at
			peer.Write(
				data.NewMetric("surprise", 0, data.UnitDimensionless, data.TimescaleTick),
				data.NewMetric("curvature", 0, data.UnitDimensionless, data.TimescaleTick),
			)

			for hIdx := range int(core.MinimumPrior) {
				values := map[string]float64{
					"surprise":  float64(2 * (hIdx % 2)),
					"curvature": float64(2 * (hIdx % 2)),
				}
				peer = peer.Next(source, values)
				peer.At, peer.From = at, at
			}

			entries := make([]*data.MetricEntry, 0, 2)

			for entry := range peer.Read() {
				entries = append(entries, entry)
			}

			return peer, entries
		}

		Convey("A frame holding only solver metrics has no evidence and emits no token", func() {
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
			So(regions.HasEvidence, ShouldBeFalse)
			So(grid.Observe(frame), ShouldBeNil)
		})

		Convey("Solver metric leakage stops scoring immediately", func() {
			sizes := [12]int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}
			frame, _ := regionFrame(grid, sizes, true)

			peer, _ := solverPeer("resonance")
			frame.Peers(peer)

			regions := grid.RegionScores(frame)
			So(regions.HasEvidence, ShouldBeFalse)
			So(grid.Observe(frame), ShouldBeNil)
		})
	})
}

func TestGrid_Dampen(t *testing.T) {
	Convey("Given a grid evaluating cross-measurement quality dampening", t, func() {
		grid := NewGrid()

		Convey("When all mature peers have positive confidence", func() {
			sizes := [12]int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}
			frame, entries := regionFrame(grid, sizes, true)
			entries[0][0].Metric.Standardized = 2.0

			regions := grid.RegionScores(frame)
			So(regions.Counts[0], ShouldEqual, 1)

			want := (2.0 - halfNormalMean) / math.Sqrt(halfNormalVariance)
			So(regions.Brightness[0], ShouldAlmostEqual, want, 1e-12)
			So(regions.HasEvidence, ShouldBeTrue)
		})

		Convey("When a peer has zero confidence (warming up), scoring does not continue and emits no evidence", func() {
			frame, entries := regionFrame(grid, [12]int{0: 1}, false)
			entries[0][0].Metric.Standardized = 5.0

			regions := grid.RegionScores(frame)
			So(regions.HasEvidence, ShouldBeFalse)
			So(grid.Observe(frame), ShouldBeNil)
		})
	})
}
