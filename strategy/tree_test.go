package strategy

import (
	"fmt"
	"strings"
	"testing"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/strategy/tokenizer"
	"github.com/theapemachine/symm/system"
)

func MakeAction(action Action) *PathBook {
	return &PathBook{
		Action:     action.String(),
		Resistance: 0.75,
	}
}

func MakeActionResistance(action Action, resistance float64) *PathBook {
	return &PathBook{
		Action:     action.String(),
		Resistance: resistance,
	}
}

func WithTree(f func(*Tree)) func() {
	return func() {
		tree := &Tree{root: iradix.New[*PathBook]()}

		for path, book := range map[string]*PathBook{
			"R06.R00":                 MakeAction(ActionEnter),
			"R06.R00/R07.R01":         MakeAction(ActionEnter),
			"R06.R00/R07.R01/R08.R02": MakeAction(ActionEnter),
			"R07.R01":                 MakeAction(ActionHold),
			"R07.R01/R08.R02":         MakeAction(ActionHold),
			"R07.R01/R08.R02/R09.R03": MakeAction(ActionHold),
			"R08.R01":                 MakeAction(ActionExit),
			"R08.R01/R09.R02":         MakeAction(ActionExit),
			"R08.R01/R09.R02/R10.R03": MakeAction(ActionExit),
			"R09.R01":                 MakeAction(ActionEnter),
			"R09.R01/R10.R02":         MakeAction(ActionHold),
			"R09.R01/R10.R02/R11.R03": MakeAction(ActionExit),
		} {
			tree.root, _, _ = tree.root.Insert([]byte(path), book)
		}

		f(tree)
	}
}

func tokensAt(prefix string, brightness float64) tokenizer.Tokens {
	tokens := tokenizer.NewTokens()

	for segment := range strings.SplitSeq(prefix, "/") {
		tokens = tokens.Append(tokenizer.NewToken([]string{segment}, brightness))
	}

	return tokens
}

func TestPredict(t *testing.T) {
	Convey("Setup", t, WithTree(func(tree *Tree) {
		Convey("When the cut is shorter than min path length", func() {
			predicted, strength, disagreed := tree.Predict(
				tokensAt("R06.R00", 1.0), FlatRegime(),
			)

			So(disagreed, ShouldBeFalse)
			So(predicted, ShouldEqual, ActionNone)
			So(strength, ShouldEqual, 0.0)
		})

		Convey("When the flat book asks for enter leaves", func() {
			for _, prefix := range []string{
				"R06.R00/R07.R01",
				"R06.R00/R07.R01/R08.R02",
			} {
				Convey(fmt.Sprintf("And brightness for %s is enough", prefix), func() {
					predicted, strength, disagreed := tree.Predict(
						tokensAt(prefix, 1.0), FlatRegime(),
					)

					So(disagreed, ShouldBeFalse)
					So(predicted, ShouldEqual, ActionEnter)
					So(strength, ShouldBeGreaterThan, 0.0)
				})

				Convey(fmt.Sprintf("And brightness for %s is not enough", prefix), func() {
					predicted, strength, disagreed := tree.Predict(
						tokensAt(prefix, 0.5), FlatRegime(),
					)

					So(disagreed, ShouldBeFalse)
					So(predicted, ShouldEqual, ActionNone)
					So(strength, ShouldEqual, 0.0)
				})
			}
		})

		Convey("When an open lot asks for hold leaves", func() {
			for _, prefix := range []string{
				"R07.R01/R08.R02",
				"R07.R01/R08.R02/R09.R03",
			} {
				Convey(fmt.Sprintf("And brightness for %s is enough", prefix), func() {
					predicted, strength, disagreed := tree.Predict(
						tokensAt(prefix, 1.0), OpenRegime(),
					)

					So(disagreed, ShouldBeFalse)
					So(predicted, ShouldEqual, ActionHold)
					So(strength, ShouldBeGreaterThan, 0.0)
				})
			}
		})

		Convey("When an open lot asks for exit leaves", func() {
			for _, prefix := range []string{
				"R08.R01/R09.R02",
				"R08.R01/R09.R02/R10.R03",
			} {
				Convey(fmt.Sprintf("And brightness for %s is enough", prefix), func() {
					predicted, strength, disagreed := tree.Predict(
						tokensAt(prefix, 1.0), OpenRegime(),
					)

					So(disagreed, ShouldBeFalse)
					So(predicted, ShouldEqual, ActionExit)
					So(strength, ShouldBeGreaterThan, 0.0)
				})
			}
		})

		Convey("When hold and exit share a prefix under an open lot", func() {
			for prefix, action := range map[string]Action{
				"R09.R01/R10.R02":         ActionNone,
				"R09.R01/R10.R02/R11.R03": ActionExit,
			} {
				predicted, strength, disagreed := tree.Predict(
					tokensAt(prefix, 1.0), OpenRegime(),
				)

				if action == ActionNone {
					So(disagreed, ShouldBeTrue)
					So(strength, ShouldEqual, 0.0)
				} else {
					So(disagreed, ShouldBeFalse)
					So(strength, ShouldBeGreaterThan, 0.0)
				}

				So(predicted, ShouldEqual, action)
			}
		})

		Convey("When the same mixed prefix is asked while flat", func() {
			predicted, strength, disagreed := tree.Predict(
				tokensAt("R09.R01/R10.R02", 1.0), FlatRegime(),
			)

			So(disagreed, ShouldBeFalse)
			So(predicted, ShouldEqual, ActionNone)
			So(strength, ShouldEqual, 0.0)
		})
	}))
}

func TestPredictRegime(t *testing.T) {
	Convey("Regime ignores out-of-band leaves under the same cut", t, func() {
		tree := &Tree{root: iradix.New[*PathBook]()}
		path := "R12.R00/R13.R01"

		tree.root, _, _ = tree.root.Insert([]byte(path), MakeAction(ActionEnter))
		tree.root, _, _ = tree.root.Insert(
			[]byte(path+"/R14.R02"), MakeAction(ActionExit),
		)

		Convey("Flat book keeps enter despite a descendant exit", func() {
			predicted, strength, disagreed := tree.Predict(
				tokensAt(path, 1.0), FlatRegime(),
			)

			So(disagreed, ShouldBeFalse)
			So(predicted, ShouldEqual, ActionEnter)
			So(strength, ShouldBeGreaterThan, 0.0)
		})

		Convey("Open lot keeps exit despite an ancestor enter", func() {
			predicted, strength, disagreed := tree.Predict(
				tokensAt(path, 1.0), OpenRegime(),
			)

			So(disagreed, ShouldBeFalse)
			So(predicted, ShouldEqual, ActionExit)
			So(strength, ShouldBeGreaterThan, 0.0)
		})
	})
}

func TestPredictPrune(t *testing.T) {
	Convey("Predict prunes high-resistance paths", t, func() {
		Convey("When no leaf counts toward the mean", func() {
			tree := &Tree{root: iradix.New[*PathBook]()}
			path := "R06.R00/R07.R01"

			tree.root, _, _ = tree.root.Insert(
				[]byte(path), MakeActionResistance(ActionEnter, 0.85),
			)

			predicted, strength, disagreed := tree.Predict(
				tokensAt(path, 1.0), FlatRegime(),
			)

			So(disagreed, ShouldBeFalse)
			So(predicted, ShouldEqual, ActionNone)
			So(strength, ShouldEqual, 0.0)
			_, ok := tree.root.Get([]byte(path))
			So(ok, ShouldBeFalse)
		})

		Convey("When a sibling still counts toward the mean", func() {
			tree := &Tree{root: iradix.New[*PathBook]()}
			heavy := "R06.R00/R07.R01"
			light := "R06.R00/R07.R01/R08.R02"

			tree.root, _, _ = tree.root.Insert(
				[]byte(heavy), MakeActionResistance(ActionEnter, 0.85),
			)
			tree.root, _, _ = tree.root.Insert([]byte(light), MakeAction(ActionEnter))

			predicted, strength, disagreed := tree.Predict(
				tokensAt(heavy, 1.0), FlatRegime(),
			)

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
		tree := &Tree{root: iradix.New[*PathBook]()}
		min := system.Cfg.Learning.MinPathLength

		So(min, ShouldBeGreaterThan, 0)

		tree.Insert("R06.R00", MakeAction(ActionEnter))
		_, ok := tree.root.Get([]byte("R06.R00"))
		So(ok, ShouldBeFalse)

		tree.Insert("R06.R00/R07.R01", MakeAction(ActionEnter))
		_, ok = tree.root.Get([]byte("R06.R00/R07.R01"))
		So(ok, ShouldBeTrue)
	})
}

func TestEnsureSeedsResistanceWithoutWipe(t *testing.T) {
	Convey("new leaves start at 0.5 and Ensure does not reset a scored book", t, func() {
		tree := &Tree{root: iradix.New[*PathBook]()}
		path := "R06.R00/R07.R01"

		tree.Ensure(path, ActionEnter)
		book, ok := tree.root.Get([]byte(path))
		So(ok, ShouldBeTrue)
		So(book.Resistance, ShouldAlmostEqual, InitialPathResistance, 0.001)
		So(PaperEligible(book.Resistance), ShouldBeFalse)

		tree.Update(path, -0.05)
		afterWin, ok := tree.root.Get([]byte(path))
		So(ok, ShouldBeTrue)
		So(afterWin.Resistance, ShouldAlmostEqual, 0.45, 0.001)
		So(PaperEligible(afterWin.Resistance), ShouldBeFalse)

		tree.Ensure(path, ActionEnter)
		kept, ok := tree.root.Get([]byte(path))
		So(ok, ShouldBeTrue)
		So(kept.Resistance, ShouldAlmostEqual, 0.45, 0.001)
	})
}

func TestPathFromStorageKey(t *testing.T) {
	Convey("PathFromStorageKey strips store prefix and action leaf", t, func() {
		So(
			PathFromStorageKey("model/paths/R01/R02/enter.bin"),
			ShouldEqual,
			"R01/R02",
		)
		So(PathFromStorageKey("model/paths/enter.bin"), ShouldEqual, "")
		So(PathFromStorageKey("other/R01/enter.bin"), ShouldEqual, "")
	})
}
