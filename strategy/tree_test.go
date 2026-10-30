package strategy

import (
	"fmt"
	"strings"
	"testing"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/symm/strategy/tokenizer"
)

func MakeAction(action Action) *data.Value {
	return data.NewValue(
		"model", "path", action.String(),
	).Append(
		0.5, "resistance",
	).Append(
		0.25, "resistance",
	)
}

func MakeActionResistance(action Action, resistance float64) *data.Value {
	return data.NewValue(
		"model", "path", action.String(),
	).Append(resistance, "resistance")
}

func WithTree(f func(*Tree)) func() {
	return func() {
		tree := &Tree{root: iradix.New[*data.Value]()}

		for path, value := range map[string]*data.Value{
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
			tree.root, _, _ = tree.root.Insert([]byte(path), value)
		}

		f(tree)
	}
}

func TestPredict(t *testing.T) {
	Convey("Setup", t, WithTree(func(tree *Tree) {
		Convey("When the prefix agrees on the action", func() {
			for prefix, action := range map[string]Action{
				"R06.R00":                 ActionEnter,
				"R06.R00/R07.R01":         ActionEnter,
				"R06.R00/R07.R01/R08.R02": ActionEnter,
				"R07.R01":                 ActionHold,
				"R07.R01/R08.R02":         ActionHold,
				"R07.R01/R08.R02/R09.R03": ActionHold,
				"R08.R01":                 ActionExit,
				"R08.R01/R09.R02":         ActionExit,
				"R08.R01/R09.R02/R10.R03": ActionExit,
			} {
				Convey(fmt.Sprintf("And the brightness for %s is enough", prefix), func() {
					tokens := tokenizer.NewTokens()
					for segment := range strings.SplitSeq(prefix, "/") {
						tokens = tokens.Append(tokenizer.NewToken([]string{segment}, 1.0))
					}

					predicted, strength, disagreed := tree.Predict(tokens)

					So(disagreed, ShouldBeFalse)
					So(predicted, ShouldEqual, action)
					So(strength, ShouldBeGreaterThan, 0.0)
				})

				Convey(fmt.Sprintf("And the brightness for %s is not enough", prefix), func() {
					tokens := tokenizer.NewTokens()
					for segment := range strings.SplitSeq(prefix, "/") {
						tokens = tokens.Append(tokenizer.NewToken([]string{segment}, 0.5))
					}

					predicted, strength, disagreed := tree.Predict(tokens)

					So(disagreed, ShouldBeFalse)
					So(predicted, ShouldEqual, ActionNone)
					So(strength, ShouldEqual, 0.0)
				})
			}
		})

		Convey("When the prefix disagrees on the action", func() {
			for prefix, action := range map[string]Action{
				"R09.R01":                 ActionNone,
				"R09.R01/R10.R02":         ActionNone,
				"R09.R01/R10.R02/R11.R03": ActionExit,
			} {
				tokens := tokenizer.NewTokens()

				for segment := range strings.SplitSeq(prefix, "/") {
					tokens = tokens.Append(tokenizer.NewToken([]string{segment}, 1.0))
				}

				predicted, strength, disagreed := tree.Predict(tokens)

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
	}))
}

func TestPredictPrune(t *testing.T) {
	Convey("Predict prunes high-resistance paths", t, func() {
		Convey("When no leaf counts toward the mean", func() {
			tree := &Tree{root: iradix.New[*data.Value]()}
			path := "R06.R00"

			tree.root, _, _ = tree.root.Insert([]byte(path), MakeActionResistance(ActionEnter, 0.85))

			tokens := tokenizer.NewTokens()
			tokens = tokens.Append(tokenizer.NewToken([]string{"R06.R00"}, 1.0))

			predicted, strength, disagreed := tree.Predict(tokens)

			So(disagreed, ShouldBeFalse)
			So(predicted, ShouldEqual, ActionNone)
			So(strength, ShouldEqual, 0.0)
			_, ok := tree.root.Get([]byte(path))
			So(ok, ShouldBeFalse)
		})

		Convey("When a sibling still counts toward the mean", func() {
			tree := &Tree{root: iradix.New[*data.Value]()}
			heavy := "R06.R00"
			light := "R06.R00/R07.R01"

			tree.root, _, _ = tree.root.Insert([]byte(heavy), MakeActionResistance(ActionEnter, 0.85))
			tree.root, _, _ = tree.root.Insert([]byte(light), MakeAction(ActionEnter))

			tokens := tokenizer.NewTokens()
			tokens = tokens.Append(tokenizer.NewToken([]string{"R06.R00"}, 1.0))

			predicted, strength, disagreed := tree.Predict(tokens)

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
