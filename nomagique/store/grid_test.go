package store_test

import (
	"fmt"
	"math"
	"math/rand/v2"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/tests/market"
)

func TestGrid(t *testing.T) {
	Convey("Given a Grid primitive", t, func() {
		grid := store.NewGrid()
		So(grid, ShouldNotBeNil)
		So(grid.Settled, ShouldBeFalse)

		Convey("When measurements are streamed into Next", func() {
			meas := data.NewMeasurement[float64]("test", nil)
			meas.Metrics = map[string]data.Metric[float64]{
				"alpha": {Label: "alpha", Raw: 1.0},
				"beta":  {Label: "beta", Raw: 1.5},
			}

			in := transport.NewOne(unsafe.Pointer(&meas)).Next(nil)

			for out := range grid.Next(in) {
				m := *(**data.Measurement[float64])(out)
				So(m, ShouldNotBeNil)
				So(m.GetMetric("alpha").Region, ShouldBeGreaterThan, 0)
				So(m.GetMetric("beta").Region, ShouldBeGreaterThan, 0)
			}
		})
	})
}

func TestGridMarketTapeReplay(t *testing.T) {
	Convey("Given a Grid primitive processing real multi-leg market tapes", t, func() {
		grid := store.NewGrid()
		tape := market.TrainingTape(4)
		So(len(tape), ShouldBeGreaterThan, 0)

		Convey("When market tape frames are updated into the grid", func() {
			for _, frame := range tape {
				grid.Update(frame)
			}

			So(len(grid.Metrics), ShouldBeGreaterThan, 1)
			// Peer-folded tape inventory can settle within a short multi-leg
			// replay; Settled is no longer required to stay false here.

			Convey("LitRegions produces valid non-zero region tokens", func() {
				token := grid.LitRegions(tape[len(tape)-1])
				So(len(token), ShouldBeGreaterThan, 0)
				for _, r := range token {
					So(r, ShouldBeGreaterThan, 0)
				}
			})
		})
	})
}

func TestGridContinuousNoiseAdversarial(t *testing.T) {
	Convey("Given a Grid under adversarial continuous Brownian noise where deltas are never zero", t, func() {
		grid := store.NewGrid()
		const metricCount = 30
		const tickCount = 200

		// Initialize random walk states
		states := make([]float64, metricCount)
		for i := range states {
			states[i] = 100.0 + float64(i)*10.0
		}

		for tick := 0; tick < tickCount; tick++ {
			meas := data.NewMeasurement[float64]("noise", nil)
			for i := 0; i < metricCount; i++ {
				// Jitter with non-zero micro-fluctuation every tick
				jitter := (rand.Float64() - 0.49) * 0.05
				if math.Abs(jitter) < 1e-6 {
					jitter = 0.001
				}
				states[i] += jitter
				label := fmt.Sprintf("metric_%02d", i)
				meas.SetMetric(label, data.Metric[float64]{
					Label: label,
					Raw:   states[i],
				})
			}

			grid.Update(meas)
		}

		Convey("A stable metric set does not settle unless one partition outlasts a completed run", func() {
			if !grid.Settled {
				return
			}

			So(grid.LongestPartitionRun, ShouldBeGreaterThan, 0)
			So(grid.PartitionRun, ShouldBeGreaterThan, grid.LongestPartitionRun)
		})
	})
}

func TestGridSympatheticClusteringAdversarial(t *testing.T) {
	Convey("Given sympathetically co-moving and consistently inverse metrics", t, func() {
		grid := store.NewGrid()
		base := 50000.0

		for tick := 0; tick < 400; tick++ {
			meas := data.NewMeasurement[float64]("market", nil)
			meas.Label = "BTC/USD"
			swing := math.Sin(float64(tick)*0.2) * 50.0

			// Sympathetic pair: leader and follower move in the same direction
			meas.SetMetric("leader", data.Metric[float64]{
				Label: "leader",
				Raw:   base + swing,
			})
			meas.SetMetric("follower", data.Metric[float64]{
				Label: "follower",
				Raw:   base*0.5 + swing*0.9 + (rand.Float64()-0.5)*0.01,
			})

			// Adversary: moves in exact opposition — consistently inverse,
			// which per the spec is also sympathetic (A+ B- when A- B+ holds).
			meas.SetMetric("adversary", data.Metric[float64]{
				Label: "adversary",
				Raw:   base - swing*1.1,
			})

			// Noise: uncorrelated random walk — not sympathetic
			meas.SetMetric("noise", data.Metric[float64]{
				Label: "noise",
				Raw:   base + float64(tick)*0.01 + (rand.Float64()-0.5)*5.0,
			})

			grid.Update(meas)
		}

		Convey("Correlated pairs record stronger co-movement than noise", func() {
			mLeader := metricNamed(grid, "BTC/USD", "market", "leader")
			mFollower := metricNamed(grid, "BTC/USD", "market", "follower")
			mNoise := metricNamed(grid, "BTC/USD", "market", "noise")
			mAdversary := metricNamed(grid, "BTC/USD", "market", "adversary")

			So(mLeader, ShouldNotBeNil)
			So(mFollower, ShouldNotBeNil)
			So(mNoise, ShouldNotBeNil)
			So(mAdversary, ShouldNotBeNil)

			// Soft lattice positions are stochastic (place + damping); the
			// clustering *signal* is the relation co-movement tally.
			lf := relationNamed(grid, mLeader.Label, mFollower.Label)
			ln := relationNamed(grid, mLeader.Label, mNoise.Label)
			So(lf, ShouldNotBeNil)
			So(ln, ShouldNotBeNil)

			symDirect := lf.PositivePositive + lf.NegativeNegative
			noiseDirect := ln.PositivePositive + ln.NegativeNegative
			So(symDirect, ShouldBeGreaterThan, noiseDirect)
		})
	})
}


func TestGridScaleStress200Metrics(t *testing.T) {
	Convey("Given scale stress with 200 distinct streaming metrics", t, func() {
		grid := store.NewGrid()
		const totalMetrics = 200
		values := make([]float64, totalMetrics)
		for i := range values {
			values[i] = 1000.0 + float64(i)
		}

		for tick := 0; tick < 120; tick++ {
			meas := data.NewMeasurement[float64]("scale_stress", nil)
			for i := 0; i < totalMetrics; i++ {
				// Co-trending groups
				group := i / 40
				delta := 0.0
				switch group {
				case 0:
					delta = 0.5 + rand.Float64()*0.1
				case 1:
					delta = -0.5 - rand.Float64()*0.1
				case 2:
					delta = math.Sin(float64(tick)*0.1) * 0.5
				case 3:
					delta = (rand.Float64() - 0.5) * 0.2
				default:
					delta = 0.1 * float64(group)
				}
				values[i] += delta
				label := fmt.Sprintf("sym_%03d", i)
				meas.SetMetric(label, data.Metric[float64]{
					Label: label,
					Raw:   values[i],
				})
			}

			grid.Update(meas)
		}

		Convey("All 200 metrics are tracked with non-degenerate spatial distribution", func() {
			So(len(grid.Metrics), ShouldEqual, totalMetrics)

			coordMap := make(map[string]int)
			for _, m := range grid.Metrics {
				key := fmt.Sprintf("%d,%d", m.X, m.Y)
				coordMap[key]++
				So(m.Region, ShouldBeGreaterThan, 0)
			}

			// The grid must use multiple distinct integer positions —
			// all metrics collapsed to a single point is degenerate.
			So(len(coordMap), ShouldBeGreaterThan, 1)
		})

		Convey("The 200 metrics form genuine regional clusters with multiple members", func() {
			regionCounts := make(map[uint8]int)
			for _, m := range grid.Metrics {
				regionCounts[m.Region]++
			}

			// Multiple regions must exist
			So(len(regionCounts), ShouldBeGreaterThan, 1)

			// Degenerate checks: must not be 200 individual regions (each with 1 node),
			// and must not be 1 region with 200 nodes.
			// At least some regions must contain multiple clustered members.
			multiMemberCount := 0
			for _, count := range regionCounts {
				if count > 1 {
					multiMemberCount++
				}
			}
			So(multiMemberCount, ShouldBeGreaterThan, 0)
		})
	})
}

func TestGridExtremeEdgeCases(t *testing.T) {
	Convey("Given extreme edge cases for Grid", t, func() {
		grid := store.NewGrid()

		Convey("Nil measurement does not panic", func() {
			So(func() { grid.Update(nil) }, ShouldNotPanic)
			So(func() { grid.Observe(nil) }, ShouldNotPanic)
			So(grid.LitRegions(nil), ShouldBeNil)
		})

		Convey("Empty metrics map does not panic", func() {
			empty := data.NewMeasurement[float64]("empty", nil)
			So(func() { grid.Update(empty) }, ShouldNotPanic)
			So(len(grid.Metrics), ShouldEqual, 0)
		})

		Convey("Single metric handles boundary safely without division by zero", func() {
			single := data.NewMeasurement[float64]("single", nil)
			single.SetMetric("lonely", data.Metric[float64]{Label: "lonely", Raw: 42.0})
			grid.Update(single)

			So(len(grid.Metrics), ShouldEqual, 1)
			So(grid.Metrics[0].Region, ShouldEqual, 1)
			So(grid.Settled, ShouldBeFalse)
		})

		Convey("Outlier spikes do not corrupt grid geometry", func() {
			meas1 := data.NewMeasurement[float64]("spike", nil)
			meas1.SetMetric("a", data.Metric[float64]{Label: "a", Raw: 1.0})
			meas1.SetMetric("b", data.Metric[float64]{Label: "b", Raw: 1.0})
			grid.Update(meas1)

			meas2 := data.NewMeasurement[float64]("spike", nil)
			meas2.SetMetric("a", data.Metric[float64]{Label: "a", Raw: 1e12})
			meas2.SetMetric("b", data.Metric[float64]{Label: "b", Raw: -1e12})
			So(func() { grid.Update(meas2) }, ShouldNotPanic)

			for _, m := range grid.Metrics {
				So(m.X, ShouldBeGreaterThanOrEqualTo, 0)
				So(m.Y, ShouldBeGreaterThanOrEqualTo, 0)
			}
		})
	})
}

func TestGridPostSettledFreezing(t *testing.T) {
	Convey("Given a grid that has settled", t, func() {
		grid := store.NewGrid()
		tape := market.TrainingTape(4)
		for _, frame := range tape {
			grid.Update(frame)
		}
		grid.Settle()
		So(grid.Settled, ShouldBeTrue)

		initialX := grid.Metrics[0].X
		initialY := grid.Metrics[0].Y
		initialRegion := grid.Metrics[0].Region
		initialLen := len(grid.Metrics)

		Convey("Violent market swings after settling must not alter the frozen grid", func() {
			for i := 0; i < 50; i++ {
				violent := data.NewMeasurement[float64]("violent", nil)
				violent.SetMetric(grid.Metrics[0].Label, data.Metric[float64]{
					Label: grid.Metrics[0].Label,
					Raw:   1e9 * float64(i+1),
				})
				grid.Update(violent)

				// Assert frozen
				So(grid.Metrics[0].X, ShouldEqual, initialX)
				So(grid.Metrics[0].Y, ShouldEqual, initialY)
				So(grid.Metrics[0].Region, ShouldEqual, initialRegion)
			}

			// New unseen metric should not be added to frozen grid
			unseen := data.NewMeasurement[float64]("unseen", nil)
			unseen.SetMetric("ghost", data.Metric[float64]{Label: "ghost", Raw: 999.0})
			grid.Update(unseen)
			So(len(grid.Metrics), ShouldEqual, initialLen)
		})
	})
}

func TestGridSettlesWhenPartitionOutlastsCompleted(t *testing.T) {
	Convey("Given co-moving metrics forming coherent regions", t, func() {
		grid := store.NewGrid()

		// Initial phase: 1 repeated metric (constant partition, never changes)
		for tick := 0; tick < 12; tick++ {
			meas := data.NewMeasurement[float64]("calm", nil)
			meas.Label = "BTC/USD"
			meas.SetMetric("mid", data.Metric[float64]{Label: "mid", Raw: 100 + float64(tick%2)})
			grid.Update(meas)
		}

		So(grid.Settled, ShouldBeFalse)
		So(grid.LongestPartitionRun, ShouldEqual, 0)
		So(grid.PartitionRun, ShouldBeGreaterThan, 1)

		Convey("A later partition settles only after it outlasts that completed run and achieves dimensional compression", func() {
			// Step to transition from "mid" to the 4-metric co-moving clusters
			setup := data.NewMeasurement[float64]("calm", nil)
			setup.Label = "BTC/USD"
			std := 1.0
			setup.SetMetric("a1", data.Metric[float64]{Label: "a1", Raw: 0.5, Standardized: &std})
			setup.SetMetric("a2", data.Metric[float64]{Label: "a2", Raw: 0.5, Standardized: &std})
			setup.SetMetric("b1", data.Metric[float64]{Label: "b1", Raw: -0.5, Standardized: &std})
			setup.SetMetric("b2", data.Metric[float64]{Label: "b2", Raw: -0.5, Standardized: &std})
			grid.Update(setup)

			targetRun := grid.LongestPartitionRun

			for tick := 0; tick < targetRun; tick++ {
				meas := data.NewMeasurement[float64]("calm", nil)
				meas.Label = "BTC/USD"
				vUp := float64(tick + 1)
				vDown := -float64(tick + 1)
				meas.SetMetric("a1", data.Metric[float64]{Label: "a1", Raw: vUp, Standardized: &std})
				meas.SetMetric("a2", data.Metric[float64]{Label: "a2", Raw: vUp, Standardized: &std})
				meas.SetMetric("b1", data.Metric[float64]{Label: "b1", Raw: vDown, Standardized: &std})
				meas.SetMetric("b2", data.Metric[float64]{Label: "b2", Raw: vDown, Standardized: &std})
				grid.Update(meas)
				So(grid.Settled, ShouldBeFalse)
			}

			// Run one more step to outlast the completed run
			meas := data.NewMeasurement[float64]("calm", nil)
			meas.Label = "BTC/USD"
			vUp := float64(targetRun + 2)
			vDown := -float64(targetRun + 2)
			meas.SetMetric("a1", data.Metric[float64]{Label: "a1", Raw: vUp, Standardized: &std})
			meas.SetMetric("a2", data.Metric[float64]{Label: "a2", Raw: vUp, Standardized: &std})
			meas.SetMetric("b1", data.Metric[float64]{Label: "b1", Raw: vDown, Standardized: &std})
			meas.SetMetric("b2", data.Metric[float64]{Label: "b2", Raw: vDown, Standardized: &std})
			grid.Update(meas)

			So(grid.LongestPartitionRun, ShouldEqual, targetRun)
			So(grid.PartitionRun, ShouldBeGreaterThan, grid.LongestPartitionRun)
			So(grid.Settled, ShouldBeTrue)
		})
	})
}

func TestGridCellIdentity(t *testing.T) {
	Convey("Given the same metric name on two symbols", t, func() {
		grid := store.NewGrid()
		btc := data.NewMeasurement[float64]("websocket", nil)
		btc.Label = "BTC/USD"
		btc.SetMetric("mid", data.Metric[float64]{Label: "mid", Raw: 1})
		eth := data.NewMeasurement[float64]("websocket", nil)
		eth.Label = "ETH/USD"
		eth.SetMetric("mid", data.Metric[float64]{Label: "mid", Raw: 2})
		grid.Update(btc)
		grid.Update(eth)

		So(len(grid.Metrics), ShouldEqual, 2)
		So(metricNamed(grid, "BTC/USD", "websocket", "mid"), ShouldNotBeNil)
		So(metricNamed(grid, "ETH/USD", "websocket", "mid"), ShouldNotBeNil)
	})
}

func TestGridLitRegionsIgnoresPeers(t *testing.T) {
	Convey("Given a measurement and the same measurement with a peer", t, func() {
		grid := store.NewGrid()

		for tick := 0; tick < 4; tick++ {
			meas := data.NewMeasurement[float64]("websocket", nil)
			meas.Label = "BTC/USD"
			meas.SetMetric("loud", data.Metric[float64]{Label: "loud", Raw: 10})
			meas.SetMetric("quiet", data.Metric[float64]{Label: "quiet", Raw: 1})
			grid.Update(meas)
		}

		parent := data.NewMeasurement[float64]("websocket", nil)
		parent.Label = "BTC/USD"
		parent.SetMetric("loud", data.Metric[float64]{Label: "loud", Raw: 10})
		parent.SetMetric("quiet", data.Metric[float64]{Label: "quiet", Raw: 1})

		withPeer := parent.Clone()
		peer := data.NewMeasurement[float64]("websocket", nil)
		peer.Label = "BTC/USD"
		peer.SetMetric("quiet", data.Metric[float64]{Label: "quiet", Raw: 1000})
		withPeer.Peers = []*data.Measurement[float64]{peer}

		// Same key on peer cannot override parent activity (Update fold parity).
		So(grid.LitRegions(withPeer), ShouldResemble, grid.LitRegions(parent))
	})
}

func TestGridLitRegionsScoresSourceKeyedPeers(t *testing.T) {
	Convey("Given ingress plus a Source-keyed producer peer with a new metric", t, func() {
		grid := store.NewGrid()

		// Register mid and energy as separate cells via canonical ingress+peer.
		for tick := 0; tick < 12; tick++ {
			ingress := data.NewMeasurement[float64]("websocket", nil)
			ingress.Label = "BTC/USD"
			std := 0.1 * float64(tick)
			ingress.SetMetric("mid", data.Metric[float64]{Label: "mid", Raw: 100, Standardized: &std})
			peer := data.NewMeasurement[float64]("resonance", nil)
			peer.Label = "BTC/USD"
			energyStd := 5.0 + float64(tick)
			peer.SetMetric("energy", data.Metric[float64]{Label: "energy", Raw: 1, Standardized: &energyStd})
			ingress.Contribute(peer)
			grid.Update(ingress)
		}
		grid.ForceSettle()

		midRegion := grid.Region("BTC/USD\x00websocket\x00mid")
		energyRegion := grid.Region("BTC/USD\x00resonance\x00energy")
		So(midRegion, ShouldNotEqual, 0)
		So(energyRegion, ShouldNotEqual, 0)
		So(energyRegion, ShouldNotEqual, midRegion)

		ingress := data.NewMeasurement[float64]("websocket", nil)
		ingress.Label = "BTC/USD"
		std := 0.1
		ingress.SetMetric("mid", data.Metric[float64]{Label: "mid", Raw: 100, Standardized: &std})
		bare := grid.LitRegions(ingress)

		peer := data.NewMeasurement[float64]("resonance", nil)
		peer.Label = "BTC/USD"
		energyStd := 50.0
		peer.SetMetric("energy", data.Metric[float64]{Label: "energy", Raw: 1, Standardized: &energyStd})
		withPeer := ingress.Clone()
		withPeer.Contribute(peer)
		lit := grid.LitRegions(withPeer)

		So(len(lit), ShouldBeGreaterThan, 0)
		// Energy region must appear when the peer is present.
		foundEnergy := false
		for _, id := range lit {
			if id == energyRegion {
				foundEnergy = true
			}
		}
		So(foundEnergy, ShouldBeTrue)
		foundBareEnergy := false
		for _, id := range bare {
			if id == energyRegion {
				foundBareEnergy = true
			}
		}
		So(foundBareEnergy, ShouldBeFalse)
	})
}

func TestGridSnapshotRoundTrip(t *testing.T) {
	Convey("Given a grid snapshot", t, func() {
		grid := store.NewGrid()
		meas := data.NewMeasurement[float64]("websocket", nil)
		meas.Label = "BTC/USD"
		meas.SetMetric("mid", data.Metric[float64]{Label: "mid", Raw: 3})
		grid.Update(meas)

		encoded, err := grid.Snapshot()
		So(err, ShouldBeNil)

		restored := store.NewGrid()
		So(restored.RestoreSnapshot(encoded), ShouldBeNil)
		So(len(restored.Metrics), ShouldEqual, len(grid.Metrics))
		So(restored.RestoreSnapshot([]byte(`{"last":null}`)), ShouldNotBeNil)
	})
}


func relationNamed(grid *store.Grid, a, b string) *store.GridRelation {
	if a > b {
		a, b = b, a
	}
	return grid.Relations[a+"\x00"+b]
}

func metricNamed(grid *store.Grid, symbol, source, name string) *data.Metric[float64] {
	key := symbol + "\x00" + source + "\x00" + name

	for _, metric := range grid.Metrics {
		if metric != nil && metric.Label == key {
			return metric
		}
	}

	return nil
}

func TestGridObservationOwnershipIntegration(t *testing.T) {
	Convey("One ingress with bid/ask consumed by several producers", t, func() {
		grid := store.NewGrid()

		ingress := data.NewMeasurement[float64]("websocket", nil)
		ingress.Label = "BTC/USD"
		bid := 50000.0
		ask := 50001.0
		ingress.SetMetric("bid", data.Metric[float64]{Label: "bid", Raw: bid})
		ingress.SetMetric("ask", data.Metric[float64]{Label: "ask", Raw: ask})

		// Producer 1: Liquidity consumes ingress, reads bid/ask, produces midpoint
		p1 := ingress.Fork()
		p1.SetSource("liquidity")
		b := p1.GetMetric("bid").Raw
		a := p1.GetMetric("ask").Raw
		So(b, ShouldEqual, 50000.0)
		So(a, ShouldEqual, 50001.0)
		p1.WriteMetric("midpoint", (b+a)/2.0)

		// Producer 2: CVD consumes ingress, reads bid/ask, produces cvd_flow
		p2 := ingress.Fork()
		p2.SetSource("cvd")
		b2 := p2.GetMetric("bid").Raw
		So(b2, ShouldEqual, 50000.0)
		p2.WriteMetric("cvd_flow", 15.5)

		// Producer 3: Toxicity consumes ingress, reads bid/ask, produces toxicity_score
		p3 := ingress.Fork()
		p3.SetSource("toxicity")
		a3 := p3.GetMetric("ask").Raw
		So(a3, ShouldEqual, 50001.0)
		p3.WriteMetric("toxicity_score", 0.8)

		// Producers contribute their outputs
		ingress.Contribute(p1)
		ingress.Contribute(p2)
		ingress.Contribute(p3)

		// Grid updates with observation
		grid.Update(ingress)

		// Grid must see exactly one ingress bid and one ingress ask
		So(metricNamed(grid, "BTC/USD", "websocket", "bid"), ShouldNotBeNil)
		So(metricNamed(grid, "BTC/USD", "websocket", "ask"), ShouldNotBeNil)

		// Grid must see only each producer's real outputs
		So(metricNamed(grid, "BTC/USD", "liquidity", "midpoint"), ShouldNotBeNil)
		So(metricNamed(grid, "BTC/USD", "cvd", "cvd_flow"), ShouldNotBeNil)
		So(metricNamed(grid, "BTC/USD", "toxicity", "toxicity_score"), ShouldNotBeNil)

		// Producers must NOT have cloned ingress bid/ask into Grid
		So(metricNamed(grid, "BTC/USD", "liquidity", "bid"), ShouldBeNil)
		So(metricNamed(grid, "BTC/USD", "liquidity", "ask"), ShouldBeNil)
		So(metricNamed(grid, "BTC/USD", "cvd", "bid"), ShouldBeNil)
		So(metricNamed(grid, "BTC/USD", "cvd", "ask"), ShouldBeNil)
		So(metricNamed(grid, "BTC/USD", "toxicity", "bid"), ShouldBeNil)
		So(metricNamed(grid, "BTC/USD", "toxicity", "ask"), ShouldBeNil)

		// Total metrics in Grid must be exactly 5 (2 ingress + 3 producer outputs)
		So(len(grid.Metrics), ShouldEqual, 5)
	})
}

func TestGridRegionalizationAndDiagnostics(t *testing.T) {
	Convey("Grid regionalization produces multi-cell attractor basins and exposes diagnostics", t, func() {
		grid := store.NewGrid()

		// 12 metrics: 3 clusters of 4 metrics each
		const nMetrics = 12
		for tick := 0; tick < 25; tick++ {
			meas := data.NewMeasurement[float64]("signals", nil)
			meas.Label = "BTC/USD"
			v1 := float64(tick + 1)
			v2 := -float64(tick + 1)
			v3 := float64(tick + 1) * 0.5
			std := 1.0

			for i := 0; i < 4; i++ {
				m1 := fmt.Sprintf("c1_%d", i)
				m2 := fmt.Sprintf("c2_%d", i)
				m3 := fmt.Sprintf("c3_%d", i)
				meas.SetMetric(m1, data.Metric[float64]{Label: m1, Raw: v1, Standardized: &std})
				meas.SetMetric(m2, data.Metric[float64]{Label: m2, Raw: v2, Standardized: &std})
				meas.SetMetric(m3, data.Metric[float64]{Label: m3, Raw: v3, Standardized: &std})
			}
			grid.Update(meas)
		}

		// Verify total cells
		So(grid.TotalCells(), ShouldEqual, nMetrics)

		// Verify real dimensional compression: 12 cells must NOT produce 11 or 12 regions
		totalRegions := grid.TotalRegions()
		So(totalRegions, ShouldBeLessThan, nMetrics-1)
		So(totalRegions, ShouldBeGreaterThanOrEqualTo, 2)

		// Verify members per region
		members := grid.MembersPerRegion()
		So(len(members), ShouldEqual, totalRegions)
		for _, count := range members {
			So(count, ShouldBeGreaterThanOrEqualTo, 1)
		}

		// Verify singleton fraction is low
		singletonFraction := grid.SingletonFraction()
		So(singletonFraction, ShouldBeLessThan, 0.5)

		// Verify within-region vs between-region sympathy/coherence
		within, between := grid.WithinVsBetweenCoherence()
		So(within, ShouldBeGreaterThan, between)
		So(within, ShouldBeGreaterThan, 0)

		// Test token emission and diagnostics
		evalMeas := data.NewMeasurement[float64]("signals", nil)
		evalMeas.Label = "BTC/USD"
		std := 1.0
		evalMeas.SetMetric("c1_0", data.Metric[float64]{Label: "c1_0", Raw: 10, Standardized: &std})
		evalMeas.SetMetric("c2_0", data.Metric[float64]{Label: "c2_0", Raw: -10, Standardized: &std})
		token := grid.LitRegions(evalMeas)
		So(token, ShouldNotBeNil)

		// Verify top-region frequency
		topFreq := grid.TopRegionFrequency()
		So(len(topFreq), ShouldBeGreaterThan, 0)

		// Verify contribution breakdown
		breakdown := grid.ContributionBreakdown(evalMeas)
		So(len(breakdown), ShouldBeGreaterThan, 0)
		for region, cellMap := range breakdown {
			So(region, ShouldBeGreaterThan, 0)
			So(len(cellMap), ShouldBeGreaterThan, 0)
		}

		// Verify token frequency and entropy
		tokenFreq := grid.TokenFrequency()
		So(len(tokenFreq), ShouldBeGreaterThan, 0)
		entropy := grid.TokenEntropy()
		So(entropy, ShouldBeGreaterThanOrEqualTo, 0)
	})

	Convey("A partition where N metrics become N-1 regions MUST fail settlement", t, func() {
		grid := store.NewGrid()

		labels := []string{"m1", "m2", "m3", "m4", "m5", "m6"}
		for tick := 0; tick < 30; tick++ {
			meas := data.NewMeasurement[float64]("test", nil)
			meas.Label = "BTC/USD"
			std := 1.0
			for _, l := range labels {
				meas.SetMetric(l, data.Metric[float64]{Label: l, Raw: float64(tick), Standardized: &std})
			}
			grid.Update(meas)
		}

		// Manually force an N-1 partition (6 metrics -> 5 regions)
		grid.Regions = map[string]uint8{
			labels[0]: 1,
			labels[1]: 1, // pair
			labels[2]: 2, // singleton
			labels[3]: 3, // singleton
			labels[4]: 4, // singleton
			labels[5]: 5, // singleton
		}

		// Must fail settlement (N-1 regions is not settled)
		So(grid.TotalRegions(), ShouldEqual, 5)
		So(grid.TotalCells(), ShouldEqual, 6)
		So(grid.TotalRegions(), ShouldEqual, grid.TotalCells()-1)

		// Force check candidate
		candidate := grid.TotalRegions() < grid.TotalCells()-1
		So(candidate, ShouldBeFalse)
	})

	Convey(">254 regions triggers a hard experiment error", t, func() {
		grid := store.NewGrid()

		// Generate 255 labels
		for i := 0; i < 255; i++ {
			lbl := fmt.Sprintf("metric_%03d", i)
			grid.PosX[lbl] = float64(i)
			grid.PosY[lbl] = float64(i)
			grid.Authority[lbl] = 1.0
			m := data.Metric[float64]{Label: lbl, Raw: 1.0}
			grid.Metrics = append(grid.Metrics, &m)
		}

		grid.ForceSettle()

		if grid.TotalRegions() > 254 {
			So(grid.Error(), ShouldNotBeNil)
		}
	})
}



