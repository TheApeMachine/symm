package strategy

import (
	"fmt"
	"strings"
	"testing"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/runtime"
)

func leaf(action Action, resistance float64) *runtime.Decision {
	return &runtime.Decision{
		Action:     action.String(),
		Resistance: resistance,
	}
}

func emptyTree() *Tree {
	tree := &Tree{}
	tree.root.Store(iradix.New[*runtime.Decision]())
	return tree
}

func put(tree *Tree, path string, decision *runtime.Decision) {
	root := tree.root.Load()
	root, _, _ = root.Insert([]byte(path), decision)
	tree.root.Store(root)
}

func get(tree *Tree, path string) (*runtime.Decision, bool) {
	return tree.root.Load().Get([]byte(path))
}

func withTree(f func(*Tree)) func() {
	return func() {
		tree := emptyTree()

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
			put(tree, path, decision)
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

		Convey("When a high-resistance leaf is the only match", func() {
			tree := emptyTree()
			path := "R06.R00/R07.R01"
			put(tree, path, leaf(ActionEnter, 0.85))

			predicted, strength, disagreed := tree.Predict(tokensAt(path, 1.0))

			So(disagreed, ShouldBeFalse)
			So(predicted, ShouldEqual, ActionNone)
			So(strength, ShouldEqual, 0.0)
			_, ok := get(tree, path)
			So(ok, ShouldBeFalse)
		})

		Convey("When a high-resistance sibling is pruned and a light leaf remains", func() {
			tree := emptyTree()
			heavy := "R06.R00/R07.R01"
			light := "R06.R00/R07.R01/R08.R02"
			put(tree, heavy, leaf(ActionEnter, 0.85))
			put(tree, light, leaf(ActionEnter, 0.75))

			predicted, strength, disagreed := tree.Predict(tokensAt(heavy, 1.0))

			So(disagreed, ShouldBeFalse)
			So(predicted, ShouldEqual, ActionEnter)
			So(strength, ShouldEqual, 0.75)
			_, ok := get(tree, heavy)
			So(ok, ShouldBeFalse)
			_, ok = get(tree, light)
			So(ok, ShouldBeTrue)
		})
	}))
}

func withUpdateLeaf(f func(*Tree, string)) func() {
	return func() {
		tree := emptyTree()
		path := "R06.R00/R07.R01"
		put(tree, path, leaf(ActionEnter, seedResistance))
		f(tree, path)
	}
}

func TestUpdate(t *testing.T) {
	Convey("Setup", t, withUpdateLeaf(func(tree *Tree, path string) {
		cases := []struct {
			name  string
			delta float64
			want  float64
		}{
			{"negative delta lowers resistance", -0.05, 0.45},
			{"positive delta raises resistance", 0.05, 0.55},
		}

		for _, tc := range cases {
			Convey("When a "+tc.name+" is applied", func() {
				fresh := emptyTree()
				put(fresh, path, leaf(ActionEnter, seedResistance))
				fresh.Update(path, tc.delta)

				decision, ok := get(fresh, path)
				So(ok, ShouldBeTrue)
				So(decision.Resistance, ShouldAlmostEqual, tc.want, 0.001)
			})
		}

		Convey("When the path is shorter than min path length", func() {
			tree.Update("R06.R00", -0.05)

			decision, ok := get(tree, path)
			So(ok, ShouldBeTrue)
			So(decision.Resistance, ShouldAlmostEqual, seedResistance, 0.001)
		})

		Convey("When the tree has no matching leaf", func() {
			bare := emptyTree()
			bare.Update(path, -0.05)

			_, ok := get(bare, path)
			So(ok, ShouldBeFalse)
		})
	}))
}

func TestEnsure(t *testing.T) {
	Convey("Ensure seeds a missing leaf once", t, func() {
		tree := emptyTree()
		path := "R06.R00/R07.R01"

		tree.Ensure(path, ActionEnter)

		decision, ok := get(tree, path)
		So(ok, ShouldBeTrue)
		So(decision.Action, ShouldEqual, "enter")
		So(decision.Resistance, ShouldAlmostEqual, seedResistance, 0.001)

		tree.Ensure(path, ActionExit)
		decision, ok = get(tree, path)
		So(ok, ShouldBeTrue)
		So(decision.Action, ShouldEqual, "enter")
		So(decision.Resistance, ShouldAlmostEqual, seedResistance, 0.001)
	})
}
