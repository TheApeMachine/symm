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
			if slices.Contains(pinned, region) {
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
	frame := data.NewMeasurement(epoch, "BTC/USD", "frame", at, at)

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

		for _, raw := range history {
			values := make(map[string]float64, sizes[idx])

			for _, label := range labels[:sizes[idx]] {
				values[label] = raw
			}

			if peer == nil {
				peer = data.NewMeasurement(epoch, "BTC/USD", source, at, at)
				metrics := make([]*data.Metric, 0, len(values))

				for label, value := range values {
					unit, timescale := data.GetDimension(source, label)
					metrics = append(metrics, data.NewMetric(label, value, unit, timescale))
				}

				peer.Write(metrics...)
				continue
			}

			peer = peer.Next(nil, values)
		}

		metricCount := 0

		for entry := range peer.Read() {
			if entry != nil && entry.Metric != nil {
				metricCount++
			}
		}

		So(metricCount, ShouldEqual, sizes[idx])
		peers[idx] = peer
	}

	frame.Peers(peers[:]...)
	frame.Write()
	return frame, peers
}

func TestGrid_Observe(t *testing.T) {
	Convey("Given a grid", t, func() {
		grid := NewGrid(context.Background())

		Convey("When every region holds metrics", func() {
			sizes := [RegionCount]int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}
			frame, _ := regionFrame(sizes, true)
			regions := grid.Observe(frame)

			Convey("It returns all regions, brightest first", func() {
				So(regions, ShouldHaveLength, RegionCount)
				So(regions[0].Brightness, ShouldBeGreaterThan, regions[RegionCount-1].Brightness)

				for idx := 1; idx < len(regions); idx++ {
					So(regions[idx].Brightness, ShouldBeLessThanOrEqualTo, regions[idx-1].Brightness)
				}
			})

			Convey("A region's brightness is the mean |SNM| of its metrics", func() {
				for _, region := range regions {
					sum := 0.0
					metricCount := 0

					for _, peer := range frame.Peers() {
						for entry := range peer.Read() {
							if entry == nil || entry.Metric == nil {
								continue
							}

							pinned := regionMetricPins[peer.Source][entry.Metric.Label()]

							if slices.Contains(pinned, region.Key) {
								sum += math.Abs(entry.Metric.SNM()) * (1 + peer.Surprise())
								metricCount++
							}
						}
					}

					So(region.Counts, ShouldEqual, metricCount)
					So(region.Brightness, ShouldAlmostEqual, sum/float64(metricCount), 1e-12)
				}
			})
		})

		Convey("When a region holds no metric", func() {
			frame, _ := regionFrame([RegionCount]int{1, 1, 1, 0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}, true)

			Convey("All regions are returned with the quiet region at zero brightness", func() {
				observed := grid.Observe(frame)
				So(observed, ShouldHaveLength, RegionCount)
				foundQuiet := false
				for _, r := range observed {
					if r.Key == 3 {
						So(r.Counts, ShouldEqual, 0)
						So(r.Brightness, ShouldEqual, 0.0)
						foundQuiet = true
					}
				}
				So(foundQuiet, ShouldBeTrue)
				So(grid.Error(), ShouldBeNil)
			})
		})

		Convey("When all regions hold no metric", func() {
			frame, _ := regionFrame([RegionCount]int{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, true)

			Convey("No regions are returned", func() {
				So(grid.Observe(frame), ShouldBeNil)
				So(grid.Error(), ShouldBeNil)
			})
		})

		Convey("When a peer carries a metric that is not pinned", func() {
			frame, _ := regionFrame([RegionCount]int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}, true)
			at := time.Unix(1_700_000_000, 0).UTC()
			solver := data.NewMeasurement(frame.Epoch, "BTC/USD", "resonance", at, at)
			solver.Write(data.NewMetric("surprise", 0, data.UnitDimensionless, data.TimescaleTick))

			for hIdx := range int(core.MinimumRankPrior) {
				solver = solver.Next(nil, map[string]float64{"surprise": float64(2 * (hIdx % 2))})
			}

			joined := data.NewMeasurement(frame.Epoch, "BTC/USD", "frame", at, at)
			joined.Peers(append(frame.Peers(), solver)...)
			joined.Write()

			Convey("The grid ignores unpinned metrics and observes the frame", func() {
				observed := grid.Observe(joined)
				So(observed, ShouldHaveLength, RegionCount)
				So(grid.Error(), ShouldBeNil)
			})
		})
	})
}
