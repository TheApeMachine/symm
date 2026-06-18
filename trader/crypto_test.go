package trader

import (
	"context"
	"encoding/json"
	"runtime"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	. "github.com/theapemachine/symm/signal"
)

func productionPool(testingTB testing.TB) *qpool.Q[any] {
	testingTB.Helper()

	pool := qpool.NewQ[any](context.Background(), 1, runtime.NumCPU(), &qpool.Config{
		SchedulingTimeout: time.Second,
		Regulators: []qpool.Regulator{
			qpool.NewRegulator(qpool.NewCircuitBreaker(10, 10*time.Second, 10)),
			qpool.NewRegulator(qpool.NewRateLimiter(100, time.Second)),
			qpool.NewRegulator(qpool.NewBackPressureRegulator(1000, time.Second, time.Second)),
			qpool.NewRegulator(qpool.NewResourceGovernorRegulator(90, 90, time.Second)),
		},
	})

	if pool == nil {
		testingTB.Fatal("qpool.NewQ returned nil")
	}

	return pool
}

func TestNewCryptoStoresSharedTree(testingTB *testing.T) {
	Convey("Given a boot tree passed into NewCrypto", testingTB, func() {
		pool := productionPool(testingTB)

		defer pool.Close()

		tree := NewTestTree()
		crypto := NewCrypto(context.Background(), pool, tree)

		defer crypto.Close()

		Convey("It should store the injected tree on the trader", func() {
			So(crypto, ShouldNotBeNil)
			So(crypto.tree, ShouldEqual, tree)
			So(crypto.story, ShouldNotBeNil)
			So(crypto.desk, ShouldNotBeNil)
		})
	})
}

func TestNewCryptoWiresResonanceSignal(testingTB *testing.T) {
	Convey("Given a crypto trader constructed with a shared tree", testingTB, func() {
		pool := productionPool(testingTB)

		defer pool.Close()

		crypto := NewCrypto(context.Background(), pool, NewTestTree())

		defer crypto.Close()

		Convey("It should construct the resonance signal", func() {
			So(crypto.resonance, ShouldNotBeNil)
		})
	})
}

func TestNewCryptoWiresCognitiveMemory(testingTB *testing.T) {
	Convey("Given a crypto trader constructed with config-backed memory", testingTB, func() {
		pool := productionPool(testingTB)

		defer pool.Close()

		viper.Set("cognitive.beam_width", 4)
		viper.Set("cognitive.beam_hops", 3)
		viper.Set("cognitive.rem_interval", time.Hour)
		defer viper.Reset()

		crypto := NewCrypto(context.Background(), pool, NewTestTree())

		defer crypto.Close()

		Convey("It should construct cognitive memory from config keys", func() {
			So(crypto.memory, ShouldNotBeNil)
		})
	})
}

func TestCryptoCollectMeasurementsFromTree(testingTB *testing.T) {
	Convey("Given measurement artifacts indexed in the shared tree", testingTB, func() {
		pool := productionPool(testingTB)

		defer pool.Close()

		tree := NewTestTree()
		crypto := NewCrypto(context.Background(), pool, tree)

		defer crypto.Close()

		artifact := datura.Acquire("fluid", datura.Artifact_Type_json).
			WithRole("measurement").
			WithScope("BTC/USD")
		artifact.WithAttribute("classifier.category", 1)
		artifact.WithAttribute("classifier.confidence", 0.72)
		artifact.WithAttribute("classifier.strength", 0.5)

		InsertMeasurement(tree, artifact)

		crypto.collectMeasurementsFromTree([]string{"BTC/USD"})

		Convey("It should hydrate story readings per symbol and source", func() {
			measurements := crypto.story.Measurements()

			So(len(measurements), ShouldEqual, 1)
			So(measurements[0].Source, ShouldEqual, logic.SourceFluid)
			So(measurements[0].Symbol, ShouldEqual, "BTC/USD")
			So(measurements[0].Category, ShouldEqual, logic.CategoryLaminar)
			So(measurements[0].Confidence, ShouldEqual, 0.72)
		})
	})
}

func TestCryptoCollectMeasurementsFromTreeScopedPrefixes(testingTB *testing.T) {
	Convey("Given measurements for multiple scopes in the tree", testingTB, func() {
		pool := productionPool(testingTB)

		defer pool.Close()

		tree := NewTestTree()
		crypto := NewCrypto(context.Background(), pool, tree)

		defer crypto.Close()

		insertTreeMeasurement := func(origin, scope string, categoryIndex int) {
			artifact := datura.Acquire(origin, datura.Artifact_Type_json).
				WithRole("measurement").
				WithScope(scope)
			artifact.WithAttribute("classifier.category", categoryIndex)
			artifact.WithAttribute("classifier.confidence", 0.72)
			artifact.WithAttribute("classifier.strength", 0.5)

			InsertMeasurement(tree, artifact)
		}

		insertTreeMeasurement("fluid", "BTC/USD", 1)
		insertTreeMeasurement("hawkes", "ETH/EUR", 2)

		crypto.collectMeasurementsFromTree([]string{"BTC/USD"})

		Convey("It should ingest only measurements under the requested scope prefix", func() {
			measurements := crypto.story.Measurements()

			So(len(measurements), ShouldEqual, 1)
			So(measurements[0].Source, ShouldEqual, logic.SourceFluid)
			So(measurements[0].Symbol, ShouldEqual, "BTC/USD")
		})
	})
}


func TestCryptoConnectSnapshotFrames(testingTB *testing.T) {
	Convey("Given a story with measurements and playbook branches", testingTB, func() {
		pool := productionPool(testingTB)

		defer pool.Close()

		crypto := NewCrypto(context.Background(), pool, NewTestTree())

		defer crypto.Close()

		measurement := logic.Measurement{
			Source:     logic.SourceHawkes,
			Symbol:     "BTC/USD",
			Category:   logic.CategorySaturation,
			Confidence: 0.8,
			Strength:   0.6,
			Price:      50000,
			Volume:     100,
			Spread:     0.2,
			Elapsed:    1,
			Surprise:   0.9,
			ObservedAt: time.Now(),
		}

		payload, _ := json.Marshal(measurement)

		So(crypto.story.Update(datura.Acquire("test", datura.Artifact_Type_json).
			WithRole("measurement").
			WithScope("BTC/USD").
			WithPayload(payload)), ShouldBeNil)

		frames := crypto.ConnectSnapshotFrames()

		Convey("It should include decision tree and state frames", func() {
			So(len(frames), ShouldBeGreaterThanOrEqualTo, 2)

			hasTree := false
			hasState := false
			var stateFrame map[string]any

			for _, frame := range frames {
				switch frame["type"] {
				case "decision_tree":
					hasTree = true
				case "state":
					hasState = true
					stateFrame = frame
				}
			}

			So(hasTree, ShouldBeTrue)
			So(hasState, ShouldBeTrue)

			gaugeReadings, ok := stateFrame["gauge_readings"].([]map[string]any)

			So(ok, ShouldBeTrue)
			So(len(gaugeReadings), ShouldEqual, 1)
			So(gaugeReadings[0]["source"], ShouldEqual, "hawkes")
		})
	})
}
