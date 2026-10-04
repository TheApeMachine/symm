package store_test

import (
	"fmt"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
)

func TestGridLifecycleAndCorrectness(t *testing.T) {
	Convey("Given a fresh Grid", t, func() {
		grid := store.NewGrid()
		So(grid.IsSettled(), ShouldBeFalse)

		Convey("First tick initializes metrics without false directional movement", func() {
			measurement := data.NewMeasurement[float64]("BTC/USD", nil)
			measurement.SetMetric("m1", data.Metric[float64]{Label: "m1", Raw: 150.0})
			measurement.SetMetric("m2", data.Metric[float64]{Label: "m2", Raw: 300.0})

			grid.Update(measurement)

			// Relations should have 0 total counts on tick 1 because neither had a prior reference value
			for _, relation := range grid.Relations {
				So(relation.Total, ShouldEqual, 0)
			}
		})

		Convey("Subsequent ticks record true directional correlation", func() {
			// Tick 1: Initialize
			meas1 := data.NewMeasurement[float64]("websocket", nil)
			meas1.Label = "BTC/USD"
			meas1.SetMetric("m1", data.Metric[float64]{Label: "m1", Raw: 100.0})
			meas1.SetMetric("m2", data.Metric[float64]{Label: "m2", Raw: 200.0})
			meas1.SetMetric("m3", data.Metric[float64]{Label: "m3", Raw: 50.0})
			grid.Update(meas1)

			// Tick 2: m1 goes UP, m2 goes UP, m3 goes DOWN
			meas2 := data.NewMeasurement[float64]("websocket", nil)
			meas2.Label = "BTC/USD"
			meas2.SetMetric("m1", data.Metric[float64]{Label: "m1", Raw: 105.0})
			meas2.SetMetric("m2", data.Metric[float64]{Label: "m2", Raw: 210.0})
			meas2.SetMetric("m3", data.Metric[float64]{Label: "m3", Raw: 40.0})
			grid.Update(meas2)

			// Tick 3: m1 goes UP, m2 goes UP, m3 goes DOWN again
			meas3 := data.NewMeasurement[float64]("websocket", nil)
			meas3.Label = "BTC/USD"
			meas3.SetMetric("m1", data.Metric[float64]{Label: "m1", Raw: 110.0})
			meas3.SetMetric("m2", data.Metric[float64]{Label: "m2", Raw: 220.0})
			meas3.SetMetric("m3", data.Metric[float64]{Label: "m3", Raw: 30.0})
			grid.Update(meas3)

			keyM1 := store.CellKey("BTC/USD", "websocket", "m1")
			keyM2 := store.CellKey("BTC/USD", "websocket", "m2")
			keyM3 := store.CellKey("BTC/USD", "websocket", "m3")

			cell1 := grid.Cells[keyM1]
			cell2 := grid.Cells[keyM2]
			cell3 := grid.Cells[keyM3]

			So(cell1, ShouldNotBeNil)
			So(cell2, ShouldNotBeNil)
			So(cell3, ShouldNotBeNil)

			rel12 := grid.Relations[(uint64(min(cell1.ID, cell2.ID))<<32)|uint64(max(cell1.ID, cell2.ID))]
			rel13 := grid.Relations[(uint64(min(cell1.ID, cell3.ID))<<32)|uint64(max(cell1.ID, cell3.ID))]

			So(rel12, ShouldNotBeNil)
			So(rel12.Same, ShouldEqual, 2)
			So(rel12.Opposite, ShouldEqual, 0)
			So(rel12.Total, ShouldEqual, 2)

			So(rel13, ShouldNotBeNil)
			So(rel13.Same, ShouldEqual, 0)
			So(rel13.Opposite, ShouldEqual, 2)
			So(rel13.Total, ShouldEqual, 2)
		})

		Convey("Peer label is not shadowed by parent symbol", func() {
			ingress := data.NewMeasurement[float64]("websocket", nil)
			ingress.Label = "BTC/USD"
			ingress.SetMetric("mid", data.Metric[float64]{Label: "mid", Raw: 50000})

			peer := data.NewMeasurement[float64]("resonance", nil)
			peer.Label = "ETH/USD"
			peer.SetMetric("mid", data.Metric[float64]{Label: "mid", Raw: 3000})
			ingress.Peers = []*data.Measurement[float64]{peer}

			grid.Update(ingress)

			btcKey := store.CellKey("BTC/USD", "websocket", "mid")
			ethKey := store.CellKey("ETH/USD", "resonance", "mid")

			So(grid.Cells[btcKey], ShouldNotBeNil)
			So(grid.Cells[ethKey], ShouldNotBeNil)
			So(btcKey, ShouldNotEqual, ethKey)
		})

		Convey("500 metrics cluster into macroscopic regions with strictly ZERO singletons", func() {
			const totalMetrics = 500
			values := make([]float64, totalMetrics)

			for index := range values {
				values[index] = 1000.0 + float64(index)
			}

			// Stream 40 cycles of synthetic market oscillations
			for tick := 0; tick < 40; tick++ {
				measurement := data.NewMeasurement[float64]("BTC/USD", nil)

				for index := 0; index < totalMetrics; index++ {
					group := index / 25
					frequency := 0.1 + float64(group)*0.03
					phase := float64(group) * 0.5
					delta := math.Sin(float64(tick)*frequency+phase) * 2.0
					values[index] += delta

					label := fmt.Sprintf("metric_%03d", index)
					measurement.SetMetric(label, data.Metric[float64]{
						Label: label,
						Raw:   values[index],
					})
				}

				grid.Update(measurement)
			}

			grid.Settle()
			So(grid.IsSettled(), ShouldBeTrue)

			// Region count should be bounded macroscopic (~15-20)
			So(len(grid.RegionMembers), ShouldBeGreaterThanOrEqualTo, 2)
			So(len(grid.RegionMembers), ShouldBeLessThanOrEqualTo, 25)

			// Zero singletons: EVERY single region must contain at least 2 members
			for regionID, memberCount := range grid.RegionMembers {
				So(memberCount, ShouldBeGreaterThanOrEqualTo, 2)
				_ = regionID
			}

			Convey("Multi-metric consensus defeats an isolated spike in LitRegions", func() {
				// Form an evaluation measurement:
				// Pick Region 1 (which has many members) and light up 15 members with activity 1.0
				// Pick Region 2 (with fewer members) and light up only 1 member with activity 2.0
				evalMeas := data.NewMeasurement[float64]("BTC/USD", nil)

				// Find members of Region 1 and Region 2
				var region1Keys, region2Keys []string

				for key, cell := range grid.Cells {
					if cell.Region == 1 {
						region1Keys = append(region1Keys, key)
					}
					if cell.Region == 2 {
						region2Keys = append(region2Keys, key)
					}
				}

				// Light up members of Region 1
				act1 := 1.0
				for index := 0; index < min(15, len(region1Keys)); index++ {
					evalMeas.SetMetric(fmt.Sprintf("r1_%d", index), data.Metric[float64]{
						Label:        region1Keys[index],
						Standardized: &act1,
					})
				}

				// Light up a single member in Region 2 with spike 2.0
				act2 := 2.0
				evalMeas.SetMetric("r2_spike", data.Metric[float64]{
					Label:        region2Keys[0],
					Standardized: &act2,
				})

				tokens := grid.LitRegions(evalMeas)
				So(len(tokens), ShouldEqual, 1)
				// The consensus in Region 1 MUST defeat the isolated spike in Region 2
				So(tokens[0][0], ShouldEqual, 1)
			})
		})

		Convey("Snapshot and Restore works cleanly and safely", func() {
			meas := data.NewMeasurement[float64]("BTC/USD", nil)
			meas.SetMetric("alpha", data.Metric[float64]{Label: "alpha", Raw: 10})
			meas.SetMetric("beta", data.Metric[float64]{Label: "beta", Raw: 20})
			grid.Update(meas)

			meas2 := data.NewMeasurement[float64]("BTC/USD", nil)
			meas2.SetMetric("alpha", data.Metric[float64]{Label: "alpha", Raw: 15})
			meas2.SetMetric("beta", data.Metric[float64]{Label: "beta", Raw: 25})
			grid.Update(meas2)

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

			for tick := 0; tick < 5; tick++ {
				ingress := data.NewMeasurement[float64]("websocket", nil)
				ingress.Label = "BTC/USD"
				ingress.SetMetric("mid", data.Metric[float64]{Label: "mid", Raw: 50000 + float64(tick)*10})

				peer := data.NewMeasurement[float64]("resonance", nil)
				peer.Label = "ETH/USD"
				peer.SetMetric("spread", data.Metric[float64]{Label: "spread", Raw: 3000 + float64(tick)*5})
				ingress.Peers = []*data.Measurement[float64]{peer}

				peerGrid.Update(ingress)
			}

			peerGrid.Settle()

			btcKey := store.CellKey("BTC/USD", "websocket", "mid")
			ethKey := store.CellKey("ETH/USD", "resonance", "spread")
			So(peerGrid.Cells[btcKey], ShouldNotBeNil)
			So(peerGrid.Cells[ethKey], ShouldNotBeNil)
			So(peerGrid.Cells[btcKey].Region, ShouldBeGreaterThan, 0)
			So(peerGrid.Cells[ethKey].Region, ShouldBeGreaterThan, 0)

			// Fire an evaluation measurement where ONLY the peer metric is active
			evalMeas := data.NewMeasurement[float64]("websocket", nil)
			evalMeas.Label = "BTC/USD"

			actVal := 3.5
			evalPeer := data.NewMeasurement[float64]("resonance", nil)
			evalPeer.Label = "ETH/USD"
			evalPeer.SetMetric("spread", data.Metric[float64]{
				Label:        "spread",
				Standardized: &actVal,
			})
			evalMeas.Peers = []*data.Measurement[float64]{evalPeer}

			tokens := peerGrid.LitRegions(evalMeas)
			So(len(tokens), ShouldEqual, 1)
			So(tokens[0][0], ShouldEqual, peerGrid.Cells[ethKey].Region)
		})

		Convey("Late arrivals are partitioned when Settle is called again", func() {
			reGrid := store.NewGrid()

			m1 := data.NewMeasurement[float64]("BTC/USD", nil)
			m1.SetMetric("alpha", data.Metric[float64]{Label: "alpha", Raw: 10})
			m1.SetMetric("beta", data.Metric[float64]{Label: "beta", Raw: 20})
			reGrid.Update(m1)
			reGrid.Settle()

			So(reGrid.IsSettled(), ShouldBeTrue)
			So(reGrid.Cells[store.CellKey("BTC/USD", "BTC/USD", "alpha")].Region, ShouldBeGreaterThan, 0)

			// Add late arrival metric
			m2 := data.NewMeasurement[float64]("BTC/USD", nil)
			m2.SetMetric("gamma", data.Metric[float64]{Label: "gamma", Raw: 30})
			reGrid.Update(m2)

			gammaKey := store.CellKey("BTC/USD", "BTC/USD", "gamma")
			So(reGrid.Cells[gammaKey], ShouldNotBeNil)
			So(reGrid.Cells[gammaKey].Region, ShouldEqual, 0)

			// Re-settle
			reGrid.Settle()
			So(reGrid.Cells[gammaKey].Region, ShouldBeGreaterThan, 0)
		})
	})
}
