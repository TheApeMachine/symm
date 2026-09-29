package impulse

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/geometry"
)

func TestEmpiricalRegionSelection(t *testing.T) {
	Convey("Region selection is empirical and not clamped to arbitrary [3, 4]", t, func() {
		market := newMarket("BTC/USD")
		market.valid = true

		// Create 6 cells in 6 distinct basins
		for i := 0; i < 6; i++ {
			cell := &Cell{
				ID:     uint64(i + 1),
				Owner:  "test",
				Metric: string(rune('a' + i)),
				Position: geometry.Point{
					X: float64(i),
					Y: float64(i),
				},
			}
			market.Cells = append(market.Cells, cell)
		}
		market.prepare()

		Convey("When only 1 region is predominantly lit, exactly 1 region is selected", func() {
			market.Cells[0].Activity = 100.0
			for i := 1; i < 6; i++ {
				market.Cells[i].Activity = 0.01
			}

			So(market.light(), ShouldBeNil)
			So(len(market.Impulse.Regions), ShouldEqual, 1)
			So(market.Impulse.Regions[0].ID, ShouldEqual, market.Cells[0].ID)
		})

		Convey("When 2 regions stand out from the rest, exactly 2 regions are selected", func() {
			market.Cells[0].Activity = 50.0
			market.Cells[1].Activity = 48.0
			for i := 2; i < 6; i++ {
				market.Cells[i].Activity = 0.5
			}

			So(market.light(), ShouldBeNil)
			So(len(market.Impulse.Regions), ShouldEqual, 2)
		})

		Convey("When 5 regions stand out from 1 cold region, exactly 5 regions are selected", func() {
			for i := 0; i < 5; i++ {
				market.Cells[i].Activity = 40.0
			}
			market.Cells[5].Activity = 0.1

			So(market.light(), ShouldBeNil)
			So(len(market.Impulse.Regions), ShouldEqual, 5)
		})
	})
}
