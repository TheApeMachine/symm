package store_test

import (
	"fmt"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/store"
)

func TestGridLifecycleAndCorrectness(t *testing.T) {
	Convey("Given a fresh Grid", t, func() {
		grid := store.NewGrid()
		So(grid.IsSettled(), ShouldBeFalse)

		key := func(name string) string {
			return store.CellKey("BTC/USD", "websocket", name)
		}

		Convey("First tick initializes metrics without false directional movement", func() {
			grid.Update(0, map[string]float64{key("m1"): 150.0, key("m2"): 300.0})

			for _, relation := range grid.Relations {
				So(relation.Total, ShouldEqual, 0)
			}
		})

		Convey("Subsequent ticks record true directional correlation", func() {
			grid.Update(0, map[string]float64{key("m1"): 100, key("m2"): 200, key("m3"): 50})
			// Tick 2: m1 goes UP, m2 goes UP, m3 goes DOWN
			grid.Update(0, map[string]float64{key("m1"): 105, key("m2"): 400, key("m3"): 40})
			// Tick 3: m1 goes UP, m2 goes UP, m3 goes DOWN again
			grid.Update(0, map[string]float64{key("m1"): 110, key("m2"): 800, key("m3"): 30})

			hasSame := false
			hasOpposite := false

			for _, relation := range grid.Relations {
				hasSame = hasSame || relation.Same > 0
				hasOpposite = hasOpposite || relation.Opposite > 0
			}

			So(hasSame, ShouldBeTrue)
			So(hasOpposite, ShouldBeTrue)
		})

		Convey("Channels from different symbols and sources stay distinct", func() {
			btcKey := store.CellKey("BTC/USD", "websocket", "mid")
			ethKey := store.CellKey("ETH/USD", "resonance", "mid")

			grid.Update(0, map[string]float64{btcKey: 50000, ethKey: 3000})

			So(grid.Cells[btcKey], ShouldNotBeNil)
			So(grid.Cells[ethKey], ShouldNotBeNil)
			So(btcKey, ShouldNotEqual, ethKey)
		})

		Convey("Unseen metrics produce no lit regions while learned metrics activate deterministically", func() {
			settleGrid := store.NewGrid()
			x1 := store.CellKey("BASE/USD", "BASE/USD", "x1")
			x2 := store.CellKey("BASE/USD", "BASE/USD", "x2")

			for tick := 0; tick < 20; tick++ {
				settleGrid.Update(0, map[string]float64{
					x1: math.Sin(float64(tick)*0.2) * 5.0,
					x2: math.Cos(float64(tick)*0.2) * 5.0,
				})
			}

			settleGrid.Settle()
			So(settleGrid.IsSettled(), ShouldBeTrue)

			tokens := settleGrid.LitRegions(map[string]float64{
				store.CellKey("SOL/USD", "SOL/USD", "depth_flow"): 150,
			})
			So(tokens, ShouldBeNil)

			learned := map[string]float64{x1: 100}
			firstTokens := settleGrid.LitRegions(learned)
			secondTokens := settleGrid.LitRegions(learned)

			So(firstTokens, ShouldNotBeNil)
			So(secondTokens, ShouldNotBeNil)
			So(firstTokens, ShouldResemble, secondTokens)
		})

		Convey("500 metrics cluster into macroscopic regions with strictly ZERO singletons", func() {
			const totalMetrics = 500
			values := make([]float64, totalMetrics)

			for index := range values {
				values[index] = 1000.0 + float64(index)
			}

			// Stream 40 cycles of synthetic market oscillations
			for tick := 0; tick < 40; tick++ {
				channels := make(map[string]float64, totalMetrics)

				for index := 0; index < totalMetrics; index++ {
					group := index / 25
					frequency := 0.1 + float64(group)*0.03
					phase := float64(group) * 0.5
					values[index] += math.Sin(float64(tick)*frequency+phase) * 2.0
					channels[key(fmt.Sprintf("metric_%03d", index))] = values[index]
				}

				grid.Update(0, channels)
			}

			grid.Settle()
			So(grid.IsSettled(), ShouldBeTrue)

			// Region count should be bounded macroscopic (~2-25)
			So(len(grid.RegionMembers), ShouldBeGreaterThanOrEqualTo, 2)
			So(len(grid.RegionMembers), ShouldBeLessThanOrEqualTo, 25)

			// Zero singletons: EVERY single region must contain at least 2 members
			for _, memberCount := range grid.RegionMembers {
				So(memberCount, ShouldBeGreaterThanOrEqualTo, 2)
			}

			var region1Keys, region2Keys []string

			for cellKey, cell := range grid.Cells {
				if cell.Region == 1 {
					region1Keys = append(region1Keys, cellKey)
				}

				if cell.Region == 2 {
					region2Keys = append(region2Keys, cellKey)
				}
			}

			So(len(region1Keys), ShouldBeGreaterThan, 0)
			So(len(region2Keys), ShouldBeGreaterThan, 0)

			// A move from L to L*(1+a)/(1-a) deforms by exactly a.
			moved := func(cellKey string, activity float64) float64 {
				return grid.Cells[cellKey].Last * (1 + activity) / (1 - activity)
			}

			Convey("RegionScores evaluates mean activity and reports coverage without sum bias", func() {
				channels := make(map[string]float64)

				// Region 1 has multiple contributing metrics with activity 0.5
				for _, cellKey := range region1Keys {
					channels[cellKey] = moved(cellKey, 0.5)
				}

				// Region 2 has a single metric with a higher individual spike 0.75
				channels[region2Keys[0]] = moved(region2Keys[0], 0.75)

				scores := grid.RegionScores(channels)
				So(len(scores), ShouldBeGreaterThanOrEqualTo, 2)

				So(scores[0].Region, ShouldEqual, 2)
				So(scores[0].Score, ShouldAlmostEqual, 0.75, 1e-9)
				So(scores[0].Contributors, ShouldEqual, 1)

				So(scores[1].Region, ShouldEqual, 1)
				So(scores[1].Score, ShouldAlmostEqual, 0.5, 1e-9)
				So(scores[1].Contributors, ShouldEqual, len(region1Keys))
				So(scores[1].Coverage, ShouldAlmostEqual, 1.0, 1e-9)

				// LitRegions conservatively returns the top-ranked region by score
				tokens := grid.LitRegions(channels)
				So(len(tokens), ShouldEqual, 1)
				So(tokens[0][0], ShouldEqual, 2)
			})

			Convey("Region with higher mean activity wins LitRegions", func() {
				channels := make(map[string]float64)

				for _, cellKey := range region1Keys {
					channels[cellKey] = moved(cellKey, 0.75)
				}

				channels[region2Keys[0]] = moved(region2Keys[0], 0.5)

				tokens := grid.LitRegions(channels)
				So(len(tokens), ShouldEqual, 1)
				So(tokens[0][0], ShouldEqual, 1)
			})
		})

		Convey("Snapshot and Restore works cleanly and safely", func() {
			grid.Update(0, map[string]float64{key("alpha"): 10, key("beta"): 20})
			grid.Update(0, map[string]float64{key("alpha"): 15, key("beta"): 25})
			grid.Settle()

			encoded, err := grid.Snapshot()
			So(err, ShouldBeNil)
			So(len(encoded), ShouldBeGreaterThan, 0)

			restoredGrid := store.NewGrid()
			err = restoredGrid.RestoreSnapshot(encoded)
			So(err, ShouldBeNil)
			So(restoredGrid.IsSettled(), ShouldBeTrue)
			So(len(restoredGrid.Cells), ShouldEqual, len(grid.Cells))
			So(len(restoredGrid.RegionMembers), ShouldEqual, len(grid.RegionMembers))
		})

		Convey("Peer metrics light up their regions in LitRegions", func() {
			peerGrid := store.NewGrid()
			btcKey := store.CellKey("BTC/USD", "websocket", "mid")
			ethKey := store.CellKey("ETH/USD", "resonance", "spread")

			for tick := 0; tick < 5; tick++ {
				peerGrid.Update(0, map[string]float64{
					btcKey: 50000 + float64(tick)*10,
					ethKey: 3000 + float64(tick)*5,
				})
			}

			peerGrid.Settle()
			So(peerGrid.IsSettled(), ShouldBeTrue)
			So(peerGrid.Cells[btcKey], ShouldNotBeNil)
			So(peerGrid.Cells[ethKey], ShouldNotBeNil)
			So(peerGrid.Cells[btcKey].Region, ShouldBeGreaterThan, 0)
			So(peerGrid.Cells[ethKey].Region, ShouldBeGreaterThan, 0)

			tokens := peerGrid.LitRegions(map[string]float64{
				ethKey: peerGrid.Cells[ethKey].Last * 3,
			})
			So(len(tokens), ShouldEqual, 1)
			So(tokens[0][0], ShouldEqual, peerGrid.Cells[ethKey].Region)
		})

		Convey("Late arrivals are partitioned when Settle is called again", func() {
			reGrid := store.NewGrid()
			alphaKey := store.CellKey("BTC/USD", "BTC/USD", "alpha")
			betaKey := store.CellKey("BTC/USD", "BTC/USD", "beta")
			gammaKey := store.CellKey("BTC/USD", "BTC/USD", "gamma")

			reGrid.Update(0, map[string]float64{alphaKey: 10, betaKey: 20})
			reGrid.Settle()

			So(reGrid.IsSettled(), ShouldBeTrue)
			So(reGrid.Cells[alphaKey].Region, ShouldBeGreaterThan, 0)

			// Add late arrival metric
			reGrid.Update(0, map[string]float64{gammaKey: 30})
			So(reGrid.Cells[gammaKey], ShouldNotBeNil)
			So(reGrid.Cells[gammaKey].Region, ShouldEqual, 0)

			// Re-settle
			reGrid.Settle()
			So(reGrid.Cells[gammaKey].Region, ShouldBeGreaterThan, 0)
		})
	})
}
