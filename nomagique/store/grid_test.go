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
			So(grid.Settled, ShouldBeTrue)

			Convey("LitRegions produces valid non-zero region tokens", func() {
				token := grid.LitRegions(tape[len(tape)-1], 3)
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

		settledAt := -1

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

			if grid.Settled && settledAt == -1 {
				settledAt = tick
			}
		}

		Convey("The grid must settle within the observation horizon despite continuous noise", func() {
			So(settledAt, ShouldBeGreaterThanOrEqualTo, 0)
			So(grid.Settled, ShouldBeTrue)
		})
	})
}

func TestGridSympatheticClusteringAdversarial(t *testing.T) {
	Convey("Given sympathetically co-moving and consistently inverse metrics", t, func() {
		grid := store.NewGrid()
		grid.SetSettlementCriteria(150, 150)
		base := 50000.0

		for tick := 0; tick < 100; tick++ {
			meas := data.NewMeasurement[float64]("market", nil)
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

		Convey("Consistently related metrics cluster closer to each other than to noise", func() {
			mLeader := grid.Metrics[0]
			mFollower := grid.Metrics[1]
			var mNoise *data.Metric[float64]

			for _, m := range grid.Metrics {
				if m.Label == "leader" {
					mLeader = m
				}
				if m.Label == "follower" {
					mFollower = m
				}
				if m.Label == "noise" {
					mNoise = m
				}
			}

			So(mNoise, ShouldNotBeNil)

			// The direct sympathetic pair (leader/follower) must be closer
			// to each other than the leader is to the uncorrelated noise.
			distSympathetic := math.Hypot(float64(mLeader.X-mFollower.X), float64(mLeader.Y-mFollower.Y))
			distNoise := math.Hypot(float64(mLeader.X-mNoise.X), float64(mLeader.Y-mNoise.Y))

			So(distSympathetic, ShouldBeLessThanOrEqualTo, distNoise)
		})
	})
}

func TestGridScaleStress200Metrics(t *testing.T) {
	Convey("Given scale stress with 200 distinct streaming metrics", t, func() {
		grid := store.NewGrid()
		grid.SetSettlementCriteria(150, 150)
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
			So(grid.LitRegions(nil, 3), ShouldBeNil)
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


