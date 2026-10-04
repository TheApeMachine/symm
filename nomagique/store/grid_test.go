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

			defAlpha := 0.5
			defBeta := 0.3
			peer1 := data.NewMeasurement("producer1", map[string]data.Metric[float64]{
				"alpha": {Raw: 5.0, Label: "alpha", Deformation: &defAlpha},
			})
			peer1.Maturity = 1.0
			peer2 := data.NewMeasurement("producer2", map[string]data.Metric[float64]{
				"beta":  {Raw: 3.2, Label: "beta", Deformation: &defBeta},
				"gamma": {Raw: 0.0, Label: "gamma"}, // inactive
			})
			peer2.Maturity = 1.0
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

		Convey("Deformation and maturity govern region lighting", func() {
			grid := NewGrid()

			Convey("A huge Raw value with Deformation == 0 does not light a region", func() {
				zeroDef := 0.0
				m := data.NewMeasurement("test", map[string]data.Metric[float64]{
					"huge": {Raw: 1e9, Label: "huge", Deformation: &zeroDef},
				})
				m.Maturity = 1.0
				tokens := grid.LitRegions(m)
				So(tokens, ShouldBeEmpty)
			})

			Convey("A tiny Raw value with nonzero Deformation does light a region", func() {
				nonzeroDef := 0.01
				m := data.NewMeasurement("test", map[string]data.Metric[float64]{
					"tiny": {Raw: 1e-9, Label: "tiny", Deformation: &nonzeroDef},
				})
				m.Maturity = 1.0
				tokens := grid.LitRegions(m)
				So(len(tokens), ShouldBeGreaterThanOrEqualTo, 1)
			})

			Convey("A metric with nil Deformation contributes nothing", func() {
				m := data.NewMeasurement("test", map[string]data.Metric[float64]{
					"nilDef": {Raw: 500.0, Label: "nilDef", Deformation: nil},
				})
				m.Maturity = 1.0
				tokens := grid.LitRegions(m)
				So(tokens, ShouldBeEmpty)
			})

			Convey("Maturity == 0 contributes nothing instead of full strength", func() {
				activeDef := 0.8
				m := data.NewMeasurement("test", map[string]data.Metric[float64]{
					"active": {Raw: 10.0, Label: "active", Deformation: &activeDef},
				})
				m.Maturity = 0.0
				tokens := grid.LitRegions(m)
				So(tokens, ShouldBeEmpty)
			})

			Convey("Measurement maturity multiplies all of that Measurement's metrics equally", func() {
				def1 := 0.5
				def2 := 0.5

				mFull := data.NewMeasurement("test", map[string]data.Metric[float64]{
					"m1": {Raw: 10.0, Label: "m1", Deformation: &def1},
					"m2": {Raw: 20.0, Label: "m2", Deformation: &def2},
				})
				mFull.Maturity = 1.0

				mHalf := data.NewMeasurement("test", map[string]data.Metric[float64]{
					"m1": {Raw: 10.0, Label: "m1", Deformation: &def1},
					"m2": {Raw: 20.0, Label: "m2", Deformation: &def2},
				})
				mHalf.Maturity = 0.5

				actFull := make(map[uint8]float64)
				assignMetricRegions(mFull, 0)
				collectRegionActivity(mFull, actFull, 0)

				actHalf := make(map[uint8]float64)
				assignMetricRegions(mHalf, 0)
				collectRegionActivity(mHalf, actHalf, 0)

				So(len(actFull), ShouldBeGreaterThan, 0)
				for r, val := range actFull {
					So(actHalf[r], ShouldAlmostEqual, val*0.5, 1e-6)
				}
			})
		})

		Convey("IsSettled transitions when measurements achieve maturity or Settle is called", func() {
			immature := data.NewMeasurement[float64]("test", nil)
			immature.Maturity = 0.5
			grid.Update(immature)
			So(grid.IsSettled(), ShouldBeFalse)

			mature := data.NewMeasurement[float64]("test", nil)
			mature.Maturity = 1.0
			grid.Update(mature)
			So(grid.IsSettled(), ShouldBeTrue)

			grid2 := NewGrid()
			grid2.Settle()
			So(grid2.IsSettled(), ShouldBeTrue)
		})
	})
}
