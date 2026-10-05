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

			for _, relation := range grid.Relations {
				So(relation.Total, ShouldEqual, 0)
			}
		})

		Convey("Subsequent ticks record true directional correlation", func() {
			firstMeasurement := data.NewMeasurement[float64]("websocket", nil)
			firstMeasurement.Label = "BTC/USD"
			firstMeasurement.SetMetric("m1", data.Metric[float64]{Label: "m1", Raw: 100.0})
			firstMeasurement.SetMetric("m2", data.Metric[float64]{Label: "m2", Raw: 200.0})
			firstMeasurement.SetMetric("m3", data.Metric[float64]{Label: "m3", Raw: 50.0})
			grid.Update(firstMeasurement)

			// Tick 2: m1 goes UP, m2 goes UP, m3 goes DOWN
			secondMeasurement := data.NewMeasurement[float64]("websocket", nil)
			secondMeasurement.Label = "BTC/USD"
			secondMeasurement.SetMetric("m1", data.Metric[float64]{Label: "m1", Raw: 105.0})
			secondMeasurement.SetMetric("m2", data.Metric[float64]{Label: "m2", Raw: 400.0})
			secondMeasurement.SetMetric("m3", data.Metric[float64]{Label: "m3", Raw: 40.0})
			grid.Update(secondMeasurement)

			// Tick 3: m1 goes UP, m2 goes UP, m3 goes DOWN again
			thirdMeasurement := data.NewMeasurement[float64]("websocket", nil)
			thirdMeasurement.Label = "BTC/USD"
			thirdMeasurement.SetMetric("m1", data.Metric[float64]{Label: "m1", Raw: 110.0})
			thirdMeasurement.SetMetric("m2", data.Metric[float64]{Label: "m2", Raw: 800.0})
			thirdMeasurement.SetMetric("m3", data.Metric[float64]{Label: "m3", Raw: 30.0})
			grid.Update(thirdMeasurement)

			hasSame := false
			hasOpposite := false
			for _, relation := range grid.Relations {
				if relation.Same > 0 {
					hasSame = true
				}

				if relation.Opposite > 0 {
					hasOpposite = true
				}
			}
			So(hasSame, ShouldBeTrue)
			So(hasOpposite, ShouldBeTrue)
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

		Convey("Unseen metrics produce no lit regions while learned metrics activate deterministically", func() {
			settleGrid := store.NewGrid()
			for tick := 0; tick < 20; tick++ {
				metricSample := data.NewMeasurement[float64]("BASE/USD", nil)
				val1 := math.Sin(float64(tick)*0.2) * 5.0
				val2 := math.Cos(float64(tick)*0.2) * 5.0
				metricSample.SetMetric("x1", data.Metric[float64]{Label: "x1", Raw: val1, Deformation: &val1})
				metricSample.SetMetric("x2", data.Metric[float64]{Label: "x2", Raw: val2, Deformation: &val2})
				settleGrid.Update(metricSample)
			}
			settleGrid.Settle()
			So(settleGrid.IsSettled(), ShouldBeTrue)

			deformation := 0.25
			unseenMeasurement := data.NewMeasurement[float64]("SOL/USD", nil)
			unseenMeasurement.Maturity = 1.0
			unseenMeasurement.SNR = 10.0
			unseenMeasurement.SNRDefined = true
			unseenMeasurement.SetMetric("depth_flow", data.Metric[float64]{Label: "depth_flow", Raw: 150, Deformation: &deformation})

			tokens := settleGrid.LitRegions(unseenMeasurement)
			So(tokens, ShouldBeNil)

			learnedMeasurement := data.NewMeasurement[float64]("BASE/USD", nil)
			learnedMeasurement.Maturity = 1.0
			learnedMeasurement.SNR = 10.0
			learnedMeasurement.SNRDefined = true
			learnedMeasurement.SetMetric("x1", data.Metric[float64]{Label: "x1", Raw: 100, Deformation: &deformation})

			firstTokens := settleGrid.LitRegions(learnedMeasurement)
			secondTokens := settleGrid.LitRegions(learnedMeasurement)

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

			// Region count should be bounded macroscopic (~2-25)
			So(len(grid.RegionMembers), ShouldBeGreaterThanOrEqualTo, 2)
			So(len(grid.RegionMembers), ShouldBeLessThanOrEqualTo, 25)

			// Zero singletons: EVERY single region must contain at least 2 members
			for _, memberCount := range grid.RegionMembers {
				So(memberCount, ShouldBeGreaterThanOrEqualTo, 2)
			}

			Convey("RegionScores evaluates mean activity and reports coverage without sum bias", func() {
				evalMeasurement := data.NewMeasurement[float64]("BTC/USD", nil)
				evalMeasurement.Maturity = 1.0
				evalMeasurement.SNR = 10.0
				evalMeasurement.SNRDefined = true

				var region1Keys, region2Keys []string
				for key, cell := range grid.Cells {
					if cell.Region == 1 {
						region1Keys = append(region1Keys, key)
					}

					if cell.Region == 2 {
						region2Keys = append(region2Keys, key)
					}
				}

				So(len(region1Keys), ShouldBeGreaterThan, 0)
				So(len(region2Keys), ShouldBeGreaterThan, 0)

				// Region 1 has multiple contributing metrics with activity 1.0
				activity1 := 1.0
				for index := 0; index < len(region1Keys); index++ {
					evalMeasurement.SetMetric(region1Keys[index], data.Metric[float64]{
						Label:       region1Keys[index],
						Deformation: &activity1,
					})
				}

				// Region 2 has a single metric with higher individual spike 2.0
				activity2 := 2.0
				evalMeasurement.SetMetric(region2Keys[0], data.Metric[float64]{
					Label:       region2Keys[0],
					Deformation: &activity2,
				})

				scores := grid.RegionScores(evalMeasurement)
				So(len(scores), ShouldBeGreaterThanOrEqualTo, 2)

				// Region 2 has higher mean score due to the 2.0 spike
				So(scores[0].Region, ShouldEqual, 2)
				So(scores[0].Score, ShouldAlmostEqual, 2.0*(10.0/11.0), 1e-6)
				So(scores[0].Contributors, ShouldEqual, 1)

				// Region 1 reports 1.0 mean score with full coverage over its members
				So(scores[1].Region, ShouldEqual, 1)
				So(scores[1].Score, ShouldAlmostEqual, 1.0*(10.0/11.0), 1e-6)
				So(scores[1].Contributors, ShouldEqual, len(region1Keys))
				So(scores[1].Coverage, ShouldAlmostEqual, 1.0, 1e-6)

				// LitRegions conservatively returns the top-ranked region by score
				tokens := grid.LitRegions(evalMeasurement)
				So(len(tokens), ShouldEqual, 1)
				So(tokens[0][0], ShouldEqual, 2)
			})

			Convey("Region with higher mean activity wins LitRegions", func() {
				evalMeasurement := data.NewMeasurement[float64]("BTC/USD", nil)
				evalMeasurement.Maturity = 1.0
				evalMeasurement.SNR = 10.0
				evalMeasurement.SNRDefined = true

				var region1Keys, region2Keys []string
				for key, cell := range grid.Cells {
					if cell.Region == 1 {
						region1Keys = append(region1Keys, key)
					}

					if cell.Region == 2 {
						region2Keys = append(region2Keys, key)
					}
				}

				activity1 := 3.0
				for index := 0; index < len(region1Keys); index++ {
					evalMeasurement.SetMetric(region1Keys[index], data.Metric[float64]{
						Label:       region1Keys[index],
						Deformation: &activity1,
					})
				}

				activity2 := 2.0
				evalMeasurement.SetMetric(region2Keys[0], data.Metric[float64]{
					Label:       region2Keys[0],
					Deformation: &activity2,
				})

				tokens := grid.LitRegions(evalMeasurement)
				So(len(tokens), ShouldEqual, 1)
				So(tokens[0][0], ShouldEqual, 1)
			})
		})

		Convey("Snapshot and Restore works cleanly and safely", func() {
			firstMeasurement := data.NewMeasurement[float64]("BTC/USD", nil)
			firstMeasurement.SetMetric("alpha", data.Metric[float64]{Label: "alpha", Raw: 10})
			firstMeasurement.SetMetric("beta", data.Metric[float64]{Label: "beta", Raw: 20})
			grid.Update(firstMeasurement)

			secondMeasurement := data.NewMeasurement[float64]("BTC/USD", nil)
			secondMeasurement.SetMetric("alpha", data.Metric[float64]{Label: "alpha", Raw: 15})
			secondMeasurement.SetMetric("beta", data.Metric[float64]{Label: "beta", Raw: 25})
			grid.Update(secondMeasurement)

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
			So(peerGrid.IsSettled(), ShouldBeTrue)

			btcKey := store.CellKey("BTC/USD", "websocket", "mid")
			ethKey := store.CellKey("ETH/USD", "resonance", "spread")
			So(peerGrid.Cells[btcKey], ShouldNotBeNil)
			So(peerGrid.Cells[ethKey], ShouldNotBeNil)
			So(peerGrid.Cells[btcKey].Region, ShouldBeGreaterThan, 0)
			So(peerGrid.Cells[ethKey].Region, ShouldBeGreaterThan, 0)

			evalMeasurement := data.NewMeasurement[float64]("websocket", nil)
			evalMeasurement.Label = "BTC/USD"
			evalMeasurement.Maturity = 1.0
			evalMeasurement.SNR = 10.0
			evalMeasurement.SNRDefined = true

			activeValue := 0.35
			evalPeer := data.NewMeasurement[float64]("resonance", nil)
			evalPeer.Label = "ETH/USD"
			evalPeer.Maturity = 1.0
			evalPeer.SNR = 10.0
			evalPeer.SNRDefined = true
			evalPeer.SetMetric("spread", data.Metric[float64]{
				Label:       "spread",
				Deformation: &activeValue,
			})
			evalMeasurement.Peers = []*data.Measurement[float64]{evalPeer}

			tokens := peerGrid.LitRegions(evalMeasurement)
			So(len(tokens), ShouldEqual, 1)
			So(tokens[0][0], ShouldEqual, peerGrid.Cells[ethKey].Region)
		})

		Convey("Late arrivals are partitioned when Settle is called again", func() {
			reGrid := store.NewGrid()

			firstMeasurement := data.NewMeasurement[float64]("BTC/USD", nil)
			firstMeasurement.SetMetric("alpha", data.Metric[float64]{Label: "alpha", Raw: 10})
			firstMeasurement.SetMetric("beta", data.Metric[float64]{Label: "beta", Raw: 20})
			reGrid.Update(firstMeasurement)
			reGrid.Settle()

			So(reGrid.IsSettled(), ShouldBeTrue)
			So(reGrid.Cells[store.CellKey("BTC/USD", "BTC/USD", "alpha")].Region, ShouldBeGreaterThan, 0)

			// Add late arrival metric
			secondMeasurement := data.NewMeasurement[float64]("BTC/USD", nil)
			secondMeasurement.SetMetric("gamma", data.Metric[float64]{Label: "gamma", Raw: 30})
			reGrid.Update(secondMeasurement)

			gammaKey := store.CellKey("BTC/USD", "BTC/USD", "gamma")
			So(reGrid.Cells[gammaKey], ShouldNotBeNil)
			So(reGrid.Cells[gammaKey].Region, ShouldEqual, 0)

			// Re-settle
			reGrid.Settle()
			So(reGrid.Cells[gammaKey].Region, ShouldBeGreaterThan, 0)
		})
	})
}
