package strategy

import (
	"fmt"
	"strings"
	"testing"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/system"
)

func leaf(action Action, resistance float64) *runtime.Decision {
	return &runtime.Decision{
		Action:     action.String(),
		Resistance: resistance,
	}
}

func withTree(f func(*Tree)) func() {
	return func() {
		tree := &Tree{root: iradix.New[*runtime.Decision]()}

		for path, decision := range map[string]*runtime.Decision{
			"R06.R00":                 leaf(ActionEnter, 0.75),
			"R06.R00/R07.R01":         leaf(ActionEnter, 0.75),
			"R06.R00/R07.R01/R08.R02": leaf(ActionEnter, 0.75),
			"R07.R01":                 leaf(ActionHold, 0.75),
			"R07.R01/R08.R02":         leaf(ActionHold, 0.75),
			"R07.R01/R08.R02/R09.R03": leaf(ActionHold, 0.75),
			"R08.R01":                 leaf(ActionExit, 0.75),
			"R08.R01/R09.R02":         leaf(ActionExit, 0.75),
			"R08.R01/R09.R02/R10.R03": leaf(ActionExit, 0.75),
			"R09.R01":                 leaf(ActionEnter, 0.75),
			"R09.R01/R10.R02":         leaf(ActionHold, 0.75),
			"R09.R01/R10.R02/R11.R03": leaf(ActionExit, 0.75),
		} {
			tree.root, _, _ = tree.root.Insert([]byte(path), decision)
		}

		f(tree)
	}
}

func tokensAt(prefix string, brightness float64) runtime.Tokens {
	tokens := runtime.NewTokens()

	for segment := range strings.SplitSeq(prefix, "/") {
		tokens = tokens.Append(runtime.NewToken([]string{segment}, brightness))
	}

	return tokens
}

func TestPredict(t *testing.T) {
	Convey("Setup", t, withTree(func(tree *Tree) {
		Convey("When the cut is shorter than min path length", func() {
			predicted, strength, disagreed := tree.Predict(tokensAt("R06.R00", 1.0))

			So(disagreed, ShouldBeFalse)
			So(predicted, ShouldEqual, ActionNone)
			So(strength, ShouldEqual, 0.0)
		})

		Convey("When enter leaves share a prefix", func() {
			for _, prefix := range []string{
				"R06.R00/R07.R01",
				"R06.R00/R07.R01/R08.R02",
			} {
				Convey(fmt.Sprintf("And brightness for %s is enough", prefix), func() {
					predicted, strength, disagreed := tree.Predict(tokensAt(prefix, 1.0))

					So(disagreed, ShouldBeFalse)
					So(predicted, ShouldEqual, ActionEnter)
					So(strength, ShouldBeGreaterThan, 0.0)
				})

				Convey(fmt.Sprintf("And brightness for %s is not enough", prefix), func() {
					predicted, strength, disagreed := tree.Predict(tokensAt(prefix, 0.5))

					So(disagreed, ShouldBeFalse)
					So(predicted, ShouldEqual, ActionNone)
					So(strength, ShouldEqual, 0.0)
				})
			}
		})

		Convey("When hold leaves share a prefix", func() {
			for _, prefix := range []string{
				"R07.R01/R08.R02",
				"R07.R01/R08.R02/R09.R03",
			} {
				Convey(fmt.Sprintf("And brightness for %s is enough", prefix), func() {
					predicted, strength, disagreed := tree.Predict(tokensAt(prefix, 1.0))

					So(disagreed, ShouldBeFalse)
					So(predicted, ShouldEqual, ActionHold)
					So(strength, ShouldBeGreaterThan, 0.0)
				})
			}
		})

		Convey("When exit leaves share a prefix", func() {
			for _, prefix := range []string{
				"R08.R01/R09.R02",
				"R08.R01/R09.R02/R10.R03",
			} {
				Convey(fmt.Sprintf("And brightness for %s is enough", prefix), func() {
					predicted, strength, disagreed := tree.Predict(tokensAt(prefix, 1.0))

					So(disagreed, ShouldBeFalse)
					So(predicted, ShouldEqual, ActionExit)
					So(strength, ShouldBeGreaterThan, 0.0)
				})
			}
		})

		Convey("When hold and exit share a prefix", func() {
			predicted, strength, disagreed := tree.Predict(
				tokensAt("R09.R01/R10.R02", 1.0),
			)

			So(disagreed, ShouldBeTrue)
			So(predicted, ShouldEqual, ActionNone)
			So(strength, ShouldEqual, 0.0)
		})
	}))
}

func TestPredictPrune(t *testing.T) {
	Convey("Predict prunes high-resistance paths", t, func() {
		Convey("When no leaf counts toward the mean", func() {
			tree := &Tree{root: iradix.New[*runtime.Decision]()}
			path := "R06.R00/R07.R01"

			tree.root, _, _ = tree.root.Insert(
				[]byte(path), leaf(ActionEnter, 0.85),
			)

			predicted, strength, disagreed := tree.Predict(tokensAt(path, 1.0))

			So(disagreed, ShouldBeFalse)
			So(predicted, ShouldEqual, ActionNone)
			So(strength, ShouldEqual, 0.0)
			_, ok := tree.root.Get([]byte(path))
			So(ok, ShouldBeFalse)
		})

		Convey("When a sibling still counts toward the mean", func() {
			tree := &Tree{root: iradix.New[*runtime.Decision]()}
			heavy := "R06.R00/R07.R01"
			light := "R06.R00/R07.R01/R08.R02"

			tree.root, _, _ = tree.root.Insert(
				[]byte(heavy), leaf(ActionEnter, 0.85),
			)
			tree.root, _, _ = tree.root.Insert(
				[]byte(light), leaf(ActionEnter, 0.75),
			)

			predicted, strength, disagreed := tree.Predict(tokensAt(heavy, 1.0))

			So(disagreed, ShouldBeFalse)
			So(predicted, ShouldEqual, ActionEnter)
			So(strength, ShouldEqual, 0.75)
			_, ok := tree.root.Get([]byte(heavy))
			So(ok, ShouldBeFalse)
			_, ok = tree.root.Get([]byte(light))
			So(ok, ShouldBeTrue)
		})
	})
}

func TestInsertMinPathLength(t *testing.T) {
	Convey("Insert rejects cuts shorter than min path length", t, func() {
		tree := &Tree{root: iradix.New[*runtime.Decision]()}
		min := system.Cfg.Learning.MinPathLength

		So(min, ShouldBeGreaterThan, 0)

		tree.Insert("R06.R00", leaf(ActionEnter, 0.75))
		_, ok := tree.root.Get([]byte("R06.R00"))
		So(ok, ShouldBeFalse)

		tree.Insert("R06.R00/R07.R01", leaf(ActionEnter, 0.75))
		_, ok = tree.root.Get([]byte("R06.R00/R07.R01"))
		So(ok, ShouldBeTrue)
	})
}

func TestEnsureSeedsResistanceWithoutWipe(t *testing.T) {
	Convey("new leaves start at seedResistance and Ensure does not reset", t, func() {
		tree := &Tree{root: iradix.New[*runtime.Decision]()}
		path := "R06.R00/R07.R01"

		tree.Ensure(path, ActionEnter)
		decision, ok := tree.root.Get([]byte(path))
		So(ok, ShouldBeTrue)
		So(decision.Resistance, ShouldAlmostEqual, seedResistance, 0.001)
		So(decision.Resistance > system.Cfg.Learning.PaperEligibleResistance, ShouldBeTrue)

		tree.Update(path, -0.05)
		afterWin, ok := tree.root.Get([]byte(path))
		So(ok, ShouldBeTrue)
		So(afterWin.Resistance, ShouldAlmostEqual, 0.45, 0.001)

		tree.Ensure(path, ActionEnter)
		kept, ok := tree.root.Get([]byte(path))
		So(ok, ShouldBeTrue)
		So(kept.Resistance, ShouldAlmostEqual, 0.45, 0.001)
	})
}
