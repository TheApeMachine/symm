package store

import (
	"context"
	"maps"
	"math"
	"slices"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/data"
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
regionFrame builds a finalized frame value with layers holding
sizes[idx] metrics pinned to that region, and returns the frame and its
peers by region.
*/
func regionFrame(sizes [RegionCount]int, warm bool) (*data.Value, [RegionCount]*data.Value) {
	var peers [RegionCount]*data.Value
	frame := data.NewValue("kraken", "frame", "BTC/USD")

	for idx := range RegionCount {
		if sizes[idx] == 0 {
			continue
		}

		source, labels := regionLabels(uint8(idx))
		So(len(labels), ShouldBeGreaterThanOrEqualTo, sizes[idx])

		val := 1.0
		if warm {
			val = float64(3 + idx)
		}

		vector := make([]float64, sizes[idx])
		for i := range vector {
			vector[i] = val
		}

		peer := data.NewValue(source, "signal", "BTC/USD")
		data.AppendVector(peer, vector, labels[:sizes[idx]]...)

		peers[idx] = peer
		frame.Layers = append(frame.Layers, peer.Layers...)
		frame.Index = append(frame.Index, peer.Index...)
	}

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

					for idx := range RegionCount {
						if sizes[idx] == 0 {
							continue
						}
						source, labels := regionLabels(uint8(idx))
						for _, label := range labels[:sizes[idx]] {
							pinned := regionMetricPins[source][label]
							if slices.Contains(pinned, region.Key) {
								sum += math.Abs(float64(3 + idx))
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
			solver := data.NewValue("resonance", "signal", "BTC/USD")
			data.AppendVector(solver, []float64{0.5}, "unpinned_mystery_metric")

			joined := data.NewValue("kraken", "frame", "BTC/USD")
			joined.Layers = append(frame.Layers, solver.Layers...)
			joined.Index = append(frame.Index, solver.Index...)

			Convey("The grid ignores unpinned metrics and observes the frame", func() {
				observed := grid.Observe(joined)
				So(observed, ShouldHaveLength, RegionCount)
				So(grid.Error(), ShouldBeNil)
			})
		})
	})
}

