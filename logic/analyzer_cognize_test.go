package logic

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura/dmt"
	pmanifold "github.com/theapemachine/nomagique/physics/fluid"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

/*
TestAnalyzerConsolidate preserves the focused unit proof for asynchronous REM
accumulation because it is independent of market ingress and signal behavior.
*/
func TestAnalyzerConsolidate(t *testing.T) {
	Convey("Given episodic observations pending behind the DMT ambiguity gate", t, func() {
		tree := dmt.NewTree("")
		analyzer := &Analyzer{tree: tree}
		sequence := []byte("symbol-btc-usd_pressure-positive")
		from := time.Unix(0, 100)
		through := time.Unix(0, 200)
		_, _, err := tree.CommitToEpisodicBuffer(uint64(from.UnixNano()), sequence)
		So(err, ShouldBeNil)
		_, _, err = tree.CommitToEpisodicBuffer(uint64(through.UnixNano()), sequence)
		So(err, ShouldBeNil)
		thesis := types.NewThesis(nil)
		thesis.Cognition.Store("BTC/USD", types.Cognition{Symbol: "BTC/USD"})

		analyzer.consolidate(thesis, []time.Time{from, through}, false)

		Convey("It should retain the interval without replaying it early", func() {
			So(tree.GetSensoryWeight(sequence).Count, ShouldEqual, 0)
			reading, _ := thesis.Cognition.Load("BTC/USD")
			So(reading.(types.Cognition).REMReplays, ShouldEqual, 0)
		})

		analyzer.consolidate(thesis, nil, true)
		analyzer.rem.Await()
		analyzer.rem.Stamp(thesis)

		Convey("It should replay the complete pending interval when requested", func() {
			So(tree.GetSensoryWeight(sequence).Count, ShouldEqual, 2)
			reading, _ := thesis.Cognition.Load("BTC/USD")
			cognition := reading.(types.Cognition)
			So(cognition.REMFrom, ShouldEqual, from)
			So(cognition.REMThrough, ShouldEqual, through)
			So(cognition.REMReplays, ShouldEqual, 2)
		})
	})
}

/*
TestAnalyzerCognize proves one buy-dominant observation publishes a Ready buy
winner after attractor training, so strategy is not gated on sequence repeats.
*/
func TestAnalyzerCognize(t *testing.T) {
	Convey("Given a gas-ready buy-dominant manifold state", t, func() {
		analyzer := &Analyzer{tree: dmt.NewTree("")}
		thesis := types.NewThesis(nil)
		state := manifold.State{
			Symbol:         "BTC/USD",
			At:             time.Unix(1, 0),
			Duration:       time.Second,
			Epoch:          1,
			ReferencePrice: decimal.NewFromInt64(100),
			InvalidReason:  manifold.Valid,
			Spread:         0.01,
			BuyCapacity:    decimal.NewFromInt64(1000),
			SellCapacity:   decimal.NewFromInt64(1000),
			BuyIntensity:   2,
			SellIntensity:  1,
			Reading: pmanifold.Reading{
				PressureGradX: 1,
				Divergence:    -1,
				CoherenceMag2: 1,
				GuidanceSpeed: 1,
			},
		}

		Convey("When cognize anchors the attractor on this sequence", func() {
			So(analyzer.cognize(thesis, state), ShouldBeTrue)

			Convey("Then Thesis cognition is Ready with a buy winner", func() {
				raw, found := thesis.Cognition.Load("BTC/USD")
				So(found, ShouldBeTrue)
				reading := raw.(types.Cognition)
				So(reading.Ready, ShouldBeTrue)
				So(reading.Winner, ShouldEqual, "buy")
				So(reading.Confidence, ShouldBeGreaterThan, 0)
			})
		})

		Convey("When REM is active on the immutable cognitive tree", func() {
			analyzer.rem = newREMSleep(t.Context(), analyzer.tree)
			analyzer.rem.mu.Lock()
			analyzer.rem.busy = true
			analyzer.rem.finished = make(chan struct{})
			analyzer.rem.mu.Unlock()

			observations, requested := analyzer.cognizeStates(
				thesis, []manifold.State{state},
			)

			So(observations, ShouldResemble, []time.Time{state.At})
			So(requested, ShouldBeFalse)
			So(thesis.Incomplete(), ShouldBeFalse)
			_, found := thesis.Cognition.Load(state.Symbol)
			So(found, ShouldBeTrue)
		})
	})
}

/*
BenchmarkAnalyzerCognize retains the narrow calculation benchmark alongside the
full production-path Analyzer benchmark.
*/
func BenchmarkAnalyzerCognize(b *testing.B) {
	analyzer := &Analyzer{tree: dmt.NewTree("")}
	state := manifold.State{
		Symbol:         "BTC/USD",
		At:             time.Unix(1, 0),
		Duration:       time.Second,
		Epoch:          1,
		ReferencePrice: decimal.NewFromInt64(100),
		InvalidReason:  manifold.Valid,
		Spread:         0.01,
		BuyCapacity:    decimal.NewFromInt64(1000),
		SellCapacity:   decimal.NewFromInt64(1000),
		BuyIntensity:   2,
		SellIntensity:  1,
		Reading: pmanifold.Reading{
			PressureGradX: 1,
			Divergence:    -1,
			CoherenceMag2: 1,
			GuidanceSpeed: 1,
		},
	}

	for b.Loop() {
		analyzer.cognize(types.NewThesis(nil), state)
	}
}
