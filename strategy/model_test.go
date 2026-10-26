package strategy

import (
	"testing"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/system"
	. "github.com/smartystreets/goconvey/convey"
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

func TestModelExplorePnLStep(t *testing.T) {
	Convey("explorePnLStep after a virtual close", t, func() {
		Convey("non-negative PnL walks the threshold down", func() {
			position := &broker.Position{Virtual: true}
			position.PnL = decimal.NewFromInt64(0)
			So(explorePnLStep(position), ShouldBeLessThan, 0)
		})

		Convey("peak open PnL counts as a win even when flat at exit", func() {
			position := &broker.Position{
				Virtual:        true,
				PeakOpenPnLPct: 0.01,
			}
			position.PnL = decimal.NewFromFloat64(-0.01)
			So(explorePnLStep(position), ShouldBeLessThan, 0)
		})

		Convey("a loss with no peak walks the threshold up", func() {
			position := &broker.Position{Virtual: true}
			position.PnL = decimal.NewFromFloat64(-0.1)
			So(explorePnLStep(position), ShouldBeGreaterThan, 0)
		})
	})
}
