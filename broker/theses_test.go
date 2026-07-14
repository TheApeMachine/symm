package broker

import (
	"net/url"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/types"
)

/*
TestNewTheses verifies that an existing store cannot be silently relabeled as
the schema understood by this binary.
*/
func TestNewTheses(t *testing.T) {
	Convey("Given a Thesis tree written by an incompatible schema", t, func() {
		tree := dmt.NewTree("")
		_, _, err := tree.Insert([]byte(thesisSchemaKey), []byte("symm/thesis/v3"))
		So(err, ShouldBeNil)

		theses, err := NewTheses(tree, nil)

		Convey("Then startup rejects the store without overwriting its marker", func() {
			So(err, ShouldNotBeNil)
			So(theses, ShouldBeNil)
			stored, found := tree.Get([]byte(thesisSchemaKey))
			So(found, ShouldBeTrue)
			So(string(stored), ShouldEqual, "symm/thesis/v3")
		})
	})
}

/*
TestThesesLoadSelectsLatest verifies that insertion order cannot let a delayed
older checkpoint replace a newer Thesis state.
*/
func TestThesesLoadSelectsLatest(t *testing.T) {
	Convey("Given a newer checkpoint inserted before an older delayed write", t, func() {
		tree := dmt.NewTree("")
		theses, err := NewTheses(tree, nil)
		So(err, ShouldBeNil)
		thesis := types.NewThesis(nil)
		thesis.Lifecycle["BTC/USD"] = types.LifecycleManaging
		thesis.RecordDecision(types.Decision{Action: "enter", Symbol: "BTC/USD"})
		older, err := thesis.MarshalBinary()
		So(err, ShouldBeNil)
		thesis.RecordDecision(types.Decision{Action: "hold", Symbol: "BTC/USD"})
		newer, err := thesis.MarshalBinary()
		So(err, ShouldBeNil)
		_, _, err = tree.Insert(theses.checkpoint("BTC/USD", 2), newer)
		So(err, ShouldBeNil)
		_, _, err = tree.Insert(theses.checkpoint("BTC/USD", 1), older)
		So(err, ShouldBeNil)

		restored, found, err := theses.Load("BTC/USD")

		Convey("Then recovery selects the timestamp rather than insertion order", func() {
			So(err, ShouldBeNil)
			So(found, ShouldBeTrue)
			So(restored.Decisions, ShouldHaveLength, 2)
			So(restored.Decisions[1].Action, ShouldEqual, "hold")
		})
	})
}

/*
TestThesesSave verifies that one active Thesis survives closing and reopening
the same persistent DMT tree.
*/
func TestThesesSave(t *testing.T) {
	Convey("Given an active position Thesis in a durable tree", t, func() {
		directory := t.TempDir()
		tree := dmt.NewTree(directory)
		theses, err := NewTheses(tree, nil)
		So(err, ShouldBeNil)

		thesis := types.NewThesis(nil)
		thesis.Decisions = append(thesis.Decisions, types.Decision{
			Action: "enter", Symbol: "BTC/USD", At: time.Unix(1, 0),
			ValidThroughEpoch: 12,
		})
		thesis.Lifecycle["BTC/USD"] = types.LifecycleManaging
		So(theses.Save("BTC/USD", thesis), ShouldBeNil)
		So(tree.Close(), ShouldBeNil)

		reopened := dmt.NewTree(directory)
		restoredStore, err := NewTheses(reopened, nil)
		So(err, ShouldBeNil)
		restored, found, err := restoredStore.Load("BTC/USD")

		Convey("Then the original strategy record is available to hydration", func() {
			So(err, ShouldBeNil)
			So(found, ShouldBeTrue)
			So(restored.LifecycleState("BTC/USD"), ShouldEqual, types.LifecycleManaging)
			So(restored.Decisions, ShouldHaveLength, 1)
			So(restored.Decisions[0].ValidThroughEpoch, ShouldEqual, uint64(12))
			So(reopened.Close(), ShouldBeNil)
		})
	})
}

/*
TestThesesLoad verifies that missing and malformed active records remain
distinguishable so recovery never replaces corruption with a blank Thesis.
*/
func TestThesesLoad(t *testing.T) {
	Convey("Given a writable Thesis tree", t, func() {
		tree := dmt.NewTree("")
		theses, err := NewTheses(tree, nil)
		So(err, ShouldBeNil)

		Convey("A symbol without an active case is reported as missing", func() {
			restored, found, err := theses.Load("BTC/USD")
			So(err, ShouldBeNil)
			So(found, ShouldBeFalse)
			So(restored, ShouldBeNil)
		})

		Convey("A malformed active case is rejected", func() {
			_, _, err := tree.Insert(theses.active("BTC/USD"), []byte("not-json"))
			So(err, ShouldBeNil)

			restored, found, err := theses.Load("BTC/USD")
			So(err, ShouldNotBeNil)
			So(found, ShouldBeTrue)
			So(restored, ShouldBeNil)
		})
	})
}

/*
TestThesesActive verifies that startup can see durable cases that are absent
from the current wallet instead of silently abandoning their lifecycle.
*/
func TestThesesActive(t *testing.T) {
	Convey("Given two durable active Theses", t, func() {
		tree := dmt.NewTree("")
		theses, err := NewTheses(tree, nil)
		So(err, ShouldBeNil)

		for _, symbol := range []string{"BTC/USD", "ETH/USD"} {
			thesis := types.NewThesis(nil)
			thesis.Lifecycle[symbol] = types.LifecycleManaging
			So(theses.Save(symbol, thesis), ShouldBeNil)
		}

		active, err := theses.Active()

		Convey("Then both symbols are restored from the active prefix", func() {
			So(err, ShouldBeNil)
			So(active, ShouldHaveLength, 2)
			So(active["BTC/USD"].LifecycleState("BTC/USD"),
				ShouldEqual, types.LifecycleManaging)
			So(active["ETH/USD"].LifecycleState("ETH/USD"),
				ShouldEqual, types.LifecycleManaging)
		})
	})
}

/*
TestThesesComplete verifies that completion archives before removing the active
record, retaining one immutable case for later PostMortem aggregation.
*/
func TestThesesComplete(t *testing.T) {
	Convey("Given an evaluated Thesis with an entry execution", t, func() {
		tree := dmt.NewTree("")
		theses, err := NewTheses(tree, nil)
		So(err, ShouldBeNil)
		thesis := types.NewThesis(nil)
		thesis.Lifecycle["BTC/USD"] = types.LifecycleEvaluated
		thesis.TradeJournal = append(thesis.TradeJournal, types.TradeObservation{
			Kind: "execution", Symbol: "BTC/USD", Side: "buy",
			ExecutionID: "entry-fill", At: time.Unix(1, 0),
		})
		So(theses.Save("BTC/USD", thesis), ShouldBeNil)

		So(theses.Complete("BTC/USD", thesis), ShouldBeNil)

		Convey("Then only the completed recovery record remains", func() {
			_, active, loadErr := theses.Load("BTC/USD")
			_, completed := tree.Get([]byte(
				"thesis/completed/" + url.PathEscape("BTC/USD") + "/entry-fill",
			))
			So(loadErr, ShouldBeNil)
			So(active, ShouldBeFalse)
			So(completed, ShouldBeTrue)
		})
	})
}

/*
BenchmarkThesesSave measures replacement of a realistic active lifecycle record
through the same DMT path used before order submission and after broker events.
*/
func BenchmarkThesesSave(b *testing.B) {
	tree := dmt.NewTree("")
	theses, err := NewTheses(tree, nil)

	if err != nil {
		b.Fatal(err)
	}

	thesis := types.NewThesis(nil)
	thesis.Lifecycle["BTC/USD"] = types.LifecycleManaging

	for index := range 256 {
		thesis.TradeJournal = append(thesis.TradeJournal, types.TradeObservation{
			Kind: "position_snapshot", Symbol: "BTC/USD", At: time.Unix(int64(index), 0),
		})
	}

	b.ReportAllocs()

	for b.Loop() {
		if err := theses.Save("BTC/USD", thesis); err != nil {
			b.Fatal(err)
		}
	}
}
