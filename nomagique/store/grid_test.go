package store

import (
	"bytes"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestGrid(t *testing.T) {
	Convey("Given a Grid", t, func() {
		grid := NewGrid()
		So(grid, ShouldNotBeNil)
		So(grid.IsSettled(), ShouldBeFalse)

		Convey("Update assigns spatial coordinates and regions to root and peers", func() {
			root := data.NewMeasurement("join", map[string]data.Metric[float64]{
				"metricA": {Raw: 1.5, Label: "metricA"},
			})

			peer := data.NewMeasurement("hawkes:trade", map[string]data.Metric[float64]{
				"hawkes_intensity": {Raw: 10.2, Label: "hawkes_intensity"},
				"zero_metric":      {Raw: 0.0, Label: "zero_metric"},
			})
			root.Peers = []*data.Measurement[float64]{peer}

			grid.Update(root)

			So(root.Metrics[0].Metric.Region, ShouldBeGreaterThanOrEqualTo, 1)
			So(root.Metrics[0].Metric.Region, ShouldBeLessThanOrEqualTo, 20)
			So(peer.Metrics[0].Metric.Region, ShouldBeGreaterThanOrEqualTo, 1)
			So(peer.Metrics[0].Metric.Region, ShouldBeLessThanOrEqualTo, 20)

			So(peer.Metrics[0].Metric.X, ShouldEqual, int64((peer.Metrics[0].Metric.Region-1)%5)*200)
			So(peer.Metrics[0].Metric.Y, ShouldEqual, int64((peer.Metrics[0].Metric.Region-1)/5)*200)
		})

		Convey("LitRegions returns deterministically sorted tokens for active metrics across peers", func() {
			root := data.NewMeasurement("join", map[string]data.Metric[float64]{})

			peer1 := data.NewMeasurement("producer1", map[string]data.Metric[float64]{
				"alpha": {Raw: 5.0, Label: "alpha"},
			})
			peer2 := data.NewMeasurement("producer2", map[string]data.Metric[float64]{
				"beta":  {Raw: 3.2, Label: "beta"},
				"gamma": {Raw: 0.0, Label: "gamma"}, // inactive
			})
			root.Peers = []*data.Measurement[float64]{peer1, peer2}

			tokens1 := grid.LitRegions(root)
			So(len(tokens1), ShouldBeGreaterThanOrEqualTo, 1)

			// Multiple invocations must produce identical token order
			for i := 0; i < 10; i++ {
				tokensN := grid.LitRegions(root)
				So(len(tokensN), ShouldEqual, len(tokens1))
				for j := range tokens1 {
					So(bytes.Equal(tokensN[j], tokens1[j]), ShouldBeTrue)
				}
			}
		})

		Convey("IsSettled transitions after 50 updates or Settle call", func() {
			for i := 0; i < 50; i++ {
				grid.Update(data.NewMeasurement[float64]("test", nil))
			}
			So(grid.IsSettled(), ShouldBeFalse)

			grid.Update(data.NewMeasurement[float64]("test", nil))
			So(grid.IsSettled(), ShouldBeTrue)

			grid2 := NewGrid()
			grid2.Settle()
			So(grid2.IsSettled(), ShouldBeTrue)
		})
	})
}
