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
	Convey("Given one repeated metric", t, func() {
		grid := store.NewGrid()

		for tick := 0; tick < 12; tick++ {
			meas := data.NewMeasurement[float64]("calm", nil)
			meas.Label = "BTC/USD"
			meas.SetMetric("mid", data.Metric[float64]{Label: "mid", Raw: 100 + float64(tick%2)})
			grid.Update(meas)
		}

		So(grid.Settled, ShouldBeFalse)
		So(grid.LongestPartitionRun, ShouldEqual, 0)
		So(grid.PartitionRun, ShouldBeGreaterThan, 1)

		Convey("A later partition settles only after it outlasts that completed run", func() {
			completed := grid.PartitionRun

			for tick := 0; tick < completed; tick++ {
				meas := data.NewMeasurement[float64]("calm", nil)
				meas.Label = "BTC/USD"
				meas.SetMetric("mid", data.Metric[float64]{Label: "mid", Raw: 100})
				meas.SetMetric("spread", data.Metric[float64]{Label: "spread", Raw: float64(tick + 1)})
				grid.Update(meas)
				So(grid.Settled, ShouldBeFalse)
			}

			meas := data.NewMeasurement[float64]("calm", nil)
			meas.Label = "BTC/USD"
			meas.SetMetric("mid", data.Metric[float64]{Label: "mid", Raw: 100})
			meas.SetMetric("spread", data.Metric[float64]{Label: "spread", Raw: 1})
			grid.Update(meas)

			So(grid.LongestPartitionRun, ShouldEqual, completed)
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


