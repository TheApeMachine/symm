package store

import (
	"context"
	"errors"
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

			peer = peer.Next(values)
		}

		metricCount := 0

		for entry := range peer.Read() {
			if entry != nil && entry.Metric != nil {
				metricCount++
			}
		}

		So(metricCount, ShouldEqual, sizes[idx])
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

			Convey("Every member is the original entry with its original provenance", func() {
				seen := make(map[*data.MetricEntry]int)

				for _, region := range regions {
					So(region.Members, ShouldHaveLength, sizes[region.Key])

					for _, member := range region.Members {
						So(member.Measurement, ShouldEqual, peers[region.Key])
						original := data.Pull(member.Measurement.Read(member.Entry.Key))
						So(member.Entry, ShouldEqual, original)
						seen[member.Entry]++
						So(member.Entry.Metric.Unit(), ShouldEqual, original.Metric.Unit())
						So(member.Entry.Metric.Timescale(), ShouldEqual, original.Metric.Timescale())
					}
				}

				for _, peer := range peers {
					for entry := range peer.Read() {
						So(seen[entry], ShouldEqual, 1)
					}
				}
			})

			Convey("A region's brightness is the mean |SNM| of its metrics", func() {
				for _, region := range regions {
					sum := 0.0
					metricCount := 0

					for entry := range peers[region.Key].Read() {
						if entry == nil || entry.Metric == nil {
							continue
						}

						sum += math.Abs(entry.Metric.SNM())
						metricCount++
					}

					So(region.Counts, ShouldEqual, sizes[region.Key])
					So(region.Brightness, ShouldAlmostEqual, sum/float64(metricCount), 1e-12)
				}
			})
		})

		Convey("When the frame is the first observation", func() {
			frame, _ := regionFrame([RegionCount]int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}, false)
			regions := grid.Observe(frame)
			So(grid.Error(), ShouldBeNil)
			So(regions, ShouldHaveLength, RegionCount)

			Convey("Zero SNM is retained, not mistaken for missing evidence", func() {
				for _, region := range regions {
					So(region.Counts, ShouldEqual, 1)
					So(region.Members, ShouldHaveLength, 1)
					So(region.Members[0].Entry.Metric.Raw(), ShouldEqual, 1)
					So(region.Members[0].Entry.Metric.SNM(), ShouldEqual, 0)
					So(region.Brightness, ShouldEqual, 0)
				}
			})
		})

		Convey("When metric values fall", func() {
			frame, peers := regionFrame([RegionCount]int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}, false)
			first := grid.Observe(frame)
			falling := data.NewMeasurement(frame.Epoch, frame.Label, "frame", 2, 2)
			falling.At, falling.From = frame.At, frame.From

			for _, peer := range peers {
				entry := data.Pull(peer.Read())
				raised := peer.Next(map[string]float64{entry.Key: 2})
				falling.Peers(raised.Next(map[string]float64{entry.Key: 1}))
			}

			regions := grid.Observe(falling.Write())
			So(grid.Error(), ShouldBeNil)
			So(regions, ShouldHaveLength, RegionCount)

			Convey("Members retain signed movement while brightness keeps its original magnitude calculation", func() {
				for _, region := range regions {
					So(region.Members[0].Entry.Metric.Raw(), ShouldEqual, 1)
					So(region.Members[0].Entry.Metric.SNM(), ShouldAlmostEqual, -1.0/3.0)
					So(region.Brightness, ShouldAlmostEqual, 1.0/3.0)
				}
			})

			Convey("Observing a successor does not overwrite previously returned members", func() {
				for _, region := range first {
					So(region.Members[0].Measurement, ShouldEqual, peers[region.Key])
					So(region.Members[0].Entry.Metric.Raw(), ShouldEqual, 1)
					So(region.Members[0].Entry.Metric.SNM(), ShouldEqual, 0)
				}
			})
		})

		Convey("When the frame is nil", func() {
			So(grid.Observe(nil), ShouldBeNil)
			So(grid.Error(), ShouldNotBeNil)
		})

		Convey("When the frame is not finalized", func() {
			frame := data.NewMeasurement(1, "BTC/USD", "frame", 1, 1)
			So(grid.Observe(frame), ShouldBeNil)
			So(grid.Error(), ShouldNotBeNil)
		})

		Convey("When a peer is nil", func() {
			frame, _ := regionFrame([RegionCount]int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}, false)
			frame.Peers(nil)
			So(grid.Observe(frame), ShouldBeNil)
			So(grid.Error(), ShouldNotBeNil)
		})

		Convey("When a producer already failed", func() {
			frame, peers := regionFrame([RegionCount]int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}, false)
			err := errors.New("producer failed")
			peers[0].SetError(err)
			So(grid.Observe(frame), ShouldBeNil)
			So(grid.Error(), ShouldNotBeNil)
			So(errors.Is(grid.Error(), err), ShouldBeTrue)
		})

		Convey("When a metric entry carries a read error", func() {
			frame, peers := regionFrame([RegionCount]int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}, false)
			err := errors.New("metric read failed")
			entry := data.Pull(peers[0].Read())
			entry.Err = err
			So(grid.Observe(frame), ShouldBeNil)
			So(grid.Error(), ShouldNotBeNil)
			So(errors.Is(grid.Error(), err), ShouldBeTrue)
		})

		Convey("When a metric entry has no metric", func() {
			frame, peers := regionFrame([RegionCount]int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}, false)
			entry := data.Pull(peers[0].Read())
			entry.Metric = nil
			So(grid.Observe(frame), ShouldBeNil)
			So(grid.Error(), ShouldNotBeNil)
		})

		Convey("When a region holds no metric", func() {
			frame, _ := regionFrame([RegionCount]int{1, 1, 1, 1, 1, 0, 1, 1, 1, 1, 1, 1}, true)

			Convey("The missing region is an error, not an ordinary empty result", func() {
				So(grid.Observe(frame), ShouldBeNil)
				So(grid.Error(), ShouldNotBeNil)
				So(grid.Error().Error(), ShouldContainSubstring, "R05")
				So(grid.Error().Error(), ShouldContainSubstring, "BTC/USD")

				complete, _ := regionFrame([RegionCount]int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}, false)
				So(grid.Observe(complete), ShouldBeNil)
			})
		})

		Convey("When a peer carries a metric that is not pinned", func() {
			frame, _ := regionFrame([RegionCount]int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}, true)
			at := time.Unix(1_700_000_000, 0).UTC()
			solver := data.NewMeasurement(frame.Epoch, "BTC/USD", "resonance", 1, 1)
			solver.At, solver.From = at, at
			solver.Write(data.NewMetric("surprise", 0, data.UnitDimensionless, data.TimescaleTick))

			for hIdx := range int(core.MinimumRankPrior) {
				solver = solver.Next(map[string]float64{"surprise": float64(2 * (hIdx % 2))})
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
