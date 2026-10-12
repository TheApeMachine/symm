package store

import (
	"context"
	"maps"
	"math"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
regionLabels returns the source with the most labels pinned to region, and
those labels in order, so fixtures use real pins.
*/
func regionLabels(region uint8) (string, []string) {
	best, labels := "", []string(nil)

	for _, source := range slices.Sorted(maps.Keys(regionMetricPins)) {
		found := make([]string, 0)

		for label, pinned := range regionMetricPins[source] {
			if pinned == region {
				found = append(found, label)
			}
		}

		if len(found) > len(labels) {
			best, labels = source, found
		}
	}

	slices.Sort(labels)
	return best, labels
}

/*
regionEpoch gives every regionFrame its own epoch, so each fixture's metric
streams start cold instead of inheriting moments from an earlier fixture.
*/
var regionEpoch atomic.Int64

/*
regionFrame builds a finalized frame with one peer per region holding
sizes[idx] metrics pinned to that region, and returns the frame and its
peers by region. When
warm is true each peer first observes core.MinimumRankPrior values through
the real finalization path, so it is past warmup; when warm is false every
peer holds its stream's first observation.
*/
func regionFrame(sizes [RegionCount]int, warm bool) (*data.Measurement, [RegionCount]*data.Measurement) {
	var peers [RegionCount]*data.Measurement
	epoch := regionEpoch.Add(1)
	at := time.Unix(1_700_000_000, 0).UTC()
	frame := data.NewMeasurement(epoch, "BTC/USD", "frame", 1, 1)
	frame.At, frame.From = at, at

	for idx := range RegionCount {
		if sizes[idx] == 0 {
			continue
		}

		source, labels := regionLabels(uint8(idx))
		So(len(labels), ShouldBeGreaterThanOrEqualTo, sizes[idx])
		history := []float64{1}

		if warm {
			history = nil

			for hIdx := range int(core.MinimumRankPrior) {
				history = append(history, float64(2*(hIdx%2)))
			}

			// Beyond every prior and distinct per region, so z-scores are
			// defined and the regions' brightnesses differ.
			history = append(history, float64(3+idx))
		}

		var peer *data.Measurement

		for hIdx, raw := range history {
			values := make(map[string]float64, sizes[idx])

			for _, label := range labels[:sizes[idx]] {
				values[label] = raw
			}

			if peer == nil {
				peer = data.NewMeasurement(epoch, "BTC/USD", source, int64(hIdx+1), int64(hIdx+1))
				peer.At, peer.From = at, at
				metrics := make([]*data.Metric, 0, len(values))

				for label, value := range values {
					unit, timescale := data.GetDimension(source, label)
					metrics = append(metrics, data.NewMetric(label, value, unit, timescale))
				}

				peer.Write(metrics...)
				continue
			}

			prior := data.NewMeasurement(epoch, "BTC/USD", "spot:trade", int64(hIdx+1), int64(hIdx+1))
			prior.At, prior.From = at, at
			peer = peer.Next(prior, values)
		}

		So(peer.Dampen(), ShouldHaveLength, sizes[idx])
		peers[idx] = peer
		frame.Peers(peer)
	}

	frame.Write()
	return frame, peers
}

func TestGrid_Observe(t *testing.T) {
	Convey("Given a grid", t, func() {
		grid := NewGrid(context.Background())

		Convey("When every region holds metrics", func() {
			sizes := [RegionCount]int{1, 1, 1, 4, 1, 1, 1, 1, 1, 1, 1, 1}
			frame, peers := regionFrame(sizes, true)
			regions := grid.Observe(frame)

			Convey("It returns all regions, brightest first", func() {
				So(regions, ShouldHaveLength, RegionCount)
				So(regions[0].Brightness, ShouldBeGreaterThan, regions[RegionCount-1].Brightness)

				for idx := 1; idx < len(regions); idx++ {
					So(regions[idx].Brightness, ShouldBeLessThanOrEqualTo, regions[idx-1].Brightness)
				}
			})

			Convey("A region's brightness is the mean |z| of its confidence-dampened metrics", func() {
				for _, region := range regions {
					dampened := peers[region.Key].Dampen()
					sum := 0.0

					for _, metric := range dampened {
						sum += math.Abs(metric.Standardized)
					}

					So(region.Counts, ShouldEqual, sizes[region.Key])
					So(region.Brightness, ShouldAlmostEqual, sum/float64(len(dampened)), 1e-12)
				}
			})
		})

		Convey("When a peer is still warming up", func() {
			frame, _ := regionFrame([RegionCount]int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}, false)

			Convey("No regions are returned", func() {
				So(grid.Observe(frame), ShouldBeNil)
				So(grid.Error(), ShouldBeNil)
			})
		})

		Convey("When a region holds no metric", func() {
			frame, _ := regionFrame([RegionCount]int{1, 1, 1, 1, 1, 0, 1, 1, 1, 1, 1, 1}, true)

			Convey("No regions are returned", func() {
				So(grid.Observe(frame), ShouldBeNil)
				So(grid.Error(), ShouldBeNil)
			})
		})

		Convey("When a peer carries a metric that is not pinned", func() {
			frame, _ := regionFrame([RegionCount]int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}, true)
			at := time.Unix(1_700_000_000, 0).UTC()
			solver := data.NewMeasurement(frame.Epoch, "BTC/USD", "resonance", 1, 1)
			solver.At, solver.From = at, at
			solver.Write(data.NewMetric("surprise", 0, data.UnitDimensionless, data.TimescaleTick))

			for hIdx := range int(core.MinimumRankPrior) {
				prior := data.NewMeasurement(frame.Epoch, "BTC/USD", "spot:trade", int64(hIdx+2), int64(hIdx+2))
				prior.At, prior.From = at, at
				solver = solver.Next(prior, map[string]float64{"surprise": float64(2 * (hIdx % 2))})
			}

			joined := data.NewMeasurement(frame.Epoch, "BTC/USD", "frame", 1, 1)
			joined.At, joined.From = at, at
			joined.Peers(append(frame.Peers(), solver)...)
			joined.Write()

			Convey("The grid reports the contract breach and observes nothing", func() {
				So(grid.Observe(joined), ShouldBeNil)
				So(grid.Error(), ShouldNotBeNil)
				So(grid.Observe(frame), ShouldBeNil)
			})
		})
	})
}
