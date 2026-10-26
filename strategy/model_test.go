package strategy

import (
	"testing"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/system"
)

func TestExploreLeafPathKeyNotSymbol(t *testing.T) {
	Convey("exploreLeaf keys by grammar path, not symbol", t, func() {
		if system.Cfg == nil {
			system.Cfg = &system.Config{Learning: system.NewLearning()}
		}

		model := &Model{explore: iradix.New[*data.Value]()}
		leaf := data.NewValue("model", "explore", "ZRC/USD").Append("enter", "action")
		leaf.Append(-0.05, "threshold")
		model.explore, _, _ = model.explore.Insert(exploreRadixKey("R07/R00", "enter"), leaf)

		got := model.exploreLeaf("R07/R00", "enter", "ZRC/USD")
		So(leafThreshold(got), ShouldAlmostEqual, 0.45, 0.001)

		wrong := model.exploreLeaf("ZRC/USD", "R07/R00", "enter")
		So(len(wrong.Index), ShouldEqual, 1)
	})
}

func TestModelExploreRadixKey(t *testing.T) {
	Convey("explore radix keys separate enter and exit at the same path", t, func() {
		enter := string(exploreRadixKey("R07.R00", "enter"))
		exit := string(exploreRadixKey("R07.R00", "exit"))
		So(enter, ShouldNotEqual, exit)
	})
}

func TestMinPathDepthBlocksPrematureExploreExit(t *testing.T) {
	Convey("stored explore exit needs a full exit prefix, not the first token", t, func() {
		if system.Cfg == nil {
			system.Cfg = &system.Config{Learning: system.NewLearning()}
		}

		model := &Model{explore: iradix.New[*data.Value]()}
		exitLeaf := data.NewValue("model", "explore", "WLD/USD").Append("exit", "action")
		model.explore, _, _ = model.explore.Insert(exploreRadixKey("R03", "exit"), exitLeaf)

		_, values, disagree := model.matchStoredPaths("R03", false, false, true)
		So(disagree, ShouldBeFalse)
		So(len(values), ShouldEqual, 1)

		So(pathDepth("R03"), ShouldEqual, 1)
		So(system.Cfg.Learning.MinPathLength, ShouldEqual, 2)
		So(pathDepth("R03") >= system.Cfg.Learning.MinPathLength, ShouldBeFalse)

		_, values, disagree = model.matchStoredPaths("R03/R04", false, false, true)
		So(disagree, ShouldBeFalse)
		So(len(values), ShouldEqual, 0)

		model.explore, _, _ = model.explore.Insert(exploreRadixKey("R03/R04", "exit"), exitLeaf)
		_, values, disagree = model.matchStoredPaths("R03/R04", false, false, true)
		So(disagree, ShouldBeFalse)
		So(len(values), ShouldEqual, 1)
		So(pathDepth("R03/R04") >= system.Cfg.Learning.MinPathLength, ShouldBeTrue)
	})
}

func TestMatchStoredPathsRequiresUnanimousAction(t *testing.T) {
	Convey("prefix matches block when enter and exit disagree", t, func() {
		if system.Cfg == nil {
			system.Cfg = &system.Config{Learning: system.NewLearning()}
		}

		model := &Model{explore: iradix.New[*data.Value]()}
		exitLeaf := data.NewValue("model", "explore", "WLD/USD").Append("exit", "action")
		enterLeaf := data.NewValue("model", "explore", "WLD/USD").Append("enter", "action")
		model.explore, _, _ = model.explore.Insert(exploreRadixKey("R03", "exit"), exitLeaf)
		model.explore, _, _ = model.explore.Insert(exploreRadixKey("R03/R04", "enter"), enterLeaf)

		_, values, disagree := model.matchStoredPaths("R03", false, false, true)
		So(disagree, ShouldBeTrue)
		So(len(values), ShouldEqual, 0)

		model = &Model{explore: iradix.New[*data.Value]()}
		model.explore, _, _ = model.explore.Insert(exploreRadixKey("R03", "exit"), exitLeaf)
		model.explore, _, _ = model.explore.Insert(exploreRadixKey("R03/R04", "exit"), exitLeaf)
		_, values, disagree = model.matchStoredPaths("R03", false, false, true)
		So(disagree, ShouldBeFalse)
		So(len(values), ShouldEqual, 2)
	})
}

func TestExploreEnterAtPrefixMatch(t *testing.T) {
	Convey("exploreEnterAt reuses a parent enter leaf on deeper paths", t, func() {
		if system.Cfg == nil {
			system.Cfg = &system.Config{Learning: system.NewLearning()}
		}

		model := &Model{explore: iradix.New[*data.Value]()}
		leaf := data.NewValue("model", "explore", "REPPO/USD").Append("enter", "action")
		model.explore, _, _ = model.explore.Insert(
			exploreRadixKey("R07.R05/R07.R03", "enter"),
			leaf,
		)

		got, ok := model.exploreEnterAt("R07.R05/R07.R03/R07.R04")
		So(ok, ShouldBeTrue)
		So(got, ShouldEqual, leaf)

		_, ok = model.exploreEnterAt("R07.R05")
		So(ok, ShouldBeFalse)
	})
}

func TestFinishVirtualExploreNewProfitDoesNotBump(t *testing.T) {
	Convey("first profitable close does not walk threshold before save", t, func() {
		if system.Cfg == nil {
			system.Cfg = &system.Config{Learning: system.NewLearning()}
		}

		model := &Model{explore: iradix.New[*data.Value](), path: NewPath()}
		position := &broker.Position{
			Virtual:   true,
			PathFound: false,
			Symbol:    "ZRC/USD",
			EntryPath: "R07/R00",
			ExitPath:  "R07/R01",
		}
		position.PnL = decimal.NewFromFloat64(1)

		model.finishVirtualExplore(position, nil)

		_, ok := model.explore.Get(exploreRadixKey("R07/R00", "enter"))
		So(ok, ShouldBeFalse)
	})
}

func TestFinishVirtualExploreIgnoresNewPathLoss(t *testing.T) {
	Convey("finishVirtualExplore skips trie writes for a new path loss", t, func() {
		if system.Cfg == nil {
			system.Cfg = &system.Config{Learning: system.NewLearning()}
		}

		model := &Model{explore: iradix.New[*data.Value](), path: NewPath()}
		position := &broker.Position{
			Virtual:   true,
			PathFound: false,
			Symbol:    "ZRC/USD",
			EntryPath: "R07/R00",
			ExitPath:  "R07/R01",
		}
		position.PnL = decimal.NewFromFloat64(-1)

		model.finishVirtualExplore(position, nil)

		_, ok := model.explore.Get(exploreRadixKey("R07/R00", "enter"))
		So(ok, ShouldBeFalse)
		So(position.ExploreSettled, ShouldBeTrue)
	})
}

func TestModelExplorePnLStep(t *testing.T) {
	Convey("explorePnLStep after a virtual close", t, func() {
		Convey("non-negative PnL walks the threshold down", func() {
			position := &broker.Position{Virtual: true}
			position.PnL = decimal.NewFromInt64(0)
			So(explorePnLStep(position), ShouldBeLessThan, 0)
		})

		Convey("large wins accelerate threshold decay", func() {
			runner := &broker.Position{Virtual: true}
			runner.PnL = decimal.NewFromFloat64(1)
			runner.PnLPct = decimal.NewFromFloat64(8.8)
			So(explorePnLStep(runner), ShouldEqual, -0.20)

			solid := &broker.Position{Virtual: true}
			solid.PnL = decimal.NewFromFloat64(1)
			solid.PnLPct = decimal.NewFromFloat64(3.0)
			So(explorePnLStep(solid), ShouldEqual, -0.10)
		})

		Convey("a loss walks the threshold up", func() {
			position := &broker.Position{Virtual: true}
			position.PnL = decimal.NewFromFloat64(-0.1)
			So(explorePnLStep(position), ShouldBeGreaterThan, 0)
		})
	})
}
