package strategy

import (
	"context"
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

func TestNewAction(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When action strings are normalized", func() {
			Convey("NewAction maps enter hold exit", func() {
				So(NewAction("enter"), ShouldEqual, ActionEnter)
				So(NewAction("hold"), ShouldEqual, ActionHold)
				So(NewAction("exit"), ShouldEqual, ActionExit)
				So(NewAction("skip"), ShouldEqual, ActionNone)
			})
		})
	})
}

func TestActionString(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When String is called on leaves", func() {
			Convey("String matches persisted action filenames", func() {
				So(ActionEnter.String(), ShouldEqual, "enter")
				So(ActionHold.String(), ShouldEqual, "hold")
				So(ActionExit.String(), ShouldEqual, "exit")
				So(ActionNone.String(), ShouldEqual, "none")
			})
		})
	})
}

func TestNewTree(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewTree boots with a nil store client", func() {
			tree := NewTree(context.Background(), "model/paths/", nil)

			Convey("Tree starts with an empty radix", func() {
				So(tree, ShouldNotBeNil)
				_, ok := tree.root.Load().Get([]byte("R06.R00/R07.R01"))
				So(ok, ShouldBeFalse)
			})
		})
	})
}

func TestPredict(t *testing.T) {
	Convey("Setup", t, withTree(func(tree *Tree) {
		Convey("When the cut is shorter than min path length", func() {
			predicted, strength, disagreed := tree.Predict(tokensAt("R06.R00", 1.0))

			Convey("Predict returns ActionNone without disagreement", func() {
				So(disagreed, ShouldBeFalse)
				So(predicted, ShouldEqual, ActionNone)
				So(strength, ShouldEqual, 0.0)
			})
		})

		Convey("When enter leaves share a prefix", func() {
			for _, prefix := range []string{
				"R06.R00/R07.R01",
				"R06.R00/R07.R01/R08.R02",
			} {
				Convey(fmt.Sprintf("When brightness for %s clears resistance", prefix), func() {
					predicted, strength, disagreed := tree.Predict(tokensAt(prefix, 1.0))

					Convey("Predict returns ActionEnter", func() {
						So(disagreed, ShouldBeFalse)
						So(predicted, ShouldEqual, ActionEnter)
						So(strength, ShouldEqual, 0.75)
					})
				})

				Convey(fmt.Sprintf("When brightness for %s is below resistance", prefix), func() {
					predicted, strength, disagreed := tree.Predict(tokensAt(prefix, 0.5))

					Convey("Predict returns ActionNone", func() {
						So(disagreed, ShouldBeFalse)
						So(predicted, ShouldEqual, ActionNone)
						So(strength, ShouldEqual, 0.0)
					})
				})
			}
		})

		Convey("When hold and exit disagree on a prefix", func() {
			predicted, strength, disagreed := tree.Predict(
				tokensAt("R09.R01/R10.R02", 1.0),
			)

			Convey("Predict disagrees", func() {
				So(disagreed, ShouldBeTrue)
				So(predicted, ShouldEqual, ActionNone)
				So(strength, ShouldEqual, 0.0)
			})
		})

		Convey("When a high-resistance leaf is the only match", func() {
			tree := emptyTree()
			path := "R06.R00/R07.R01"
			put(tree, path, leaf(ActionEnter, 0.85))

			predicted, strength, disagreed := tree.Predict(tokensAt(path, 1.0))

			Convey("Leaf is pruned and Predict returns ActionNone", func() {
				So(disagreed, ShouldBeFalse)
				So(predicted, ShouldEqual, ActionNone)
				So(strength, ShouldEqual, 0.0)
				_, ok := get(tree, path)
				So(ok, ShouldBeFalse)
			})
		})
	}))
}

func TestUpdate(t *testing.T) {
	Convey("Setup", t, func() {
		path := "R06.R00/R07.R01"
		start := seedResistance()

		Convey("When a negative delta is applied", func() {
			tree := emptyTree()
			put(tree, path, leaf(ActionEnter, start))
			tree.Update(path, -0.05)

			decision, ok := get(tree, path)

			Convey("Resistance drops to 0.45", func() {
				So(ok, ShouldBeTrue)
				So(decision.Resistance, ShouldEqual, 0.45)
			})
		})

		Convey("When a positive delta is applied", func() {
			tree := emptyTree()
			put(tree, path, leaf(ActionEnter, start))
			tree.Update(path, 0.05)

			decision, ok := get(tree, path)

			Convey("Resistance rises to 0.55", func() {
				So(ok, ShouldBeTrue)
				So(decision.Resistance, ShouldEqual, 0.55)
			})
		})

		Convey("When the path is shorter than min path length", func() {
			tree := emptyTree()
			put(tree, path, leaf(ActionEnter, start))
			tree.Update("R06.R00", -0.05)

			decision, ok := get(tree, path)

			Convey("Resistance is unchanged", func() {
				So(ok, ShouldBeTrue)
				So(decision.Resistance, ShouldEqual, start)
			})
		})
	})
}

func TestInsert(t *testing.T) {
	Convey("Setup", t, func() {
		tree := emptyTree()
		path := "R06.R00/R07.R01"
		decision := leaf(ActionEnter, 0.42)

		Convey("When Insert is called with a full path", func() {
			tree.Insert(path, decision)

			got, ok := get(tree, path)

			Convey("Leaf is stored", func() {
				So(ok, ShouldBeTrue)
				So(got.Action, ShouldEqual, "enter")
				So(got.Resistance, ShouldEqual, 0.42)
			})
		})

		Convey("When path or decision is missing", func() {
			tree.Insert("", decision)
			tree.Insert(path, nil)

			_, ok := get(tree, path)

			Convey("Insert is a no-op", func() {
				So(ok, ShouldBeFalse)
			})
		})
	})
}

func TestEnsure(t *testing.T) {
	Convey("Setup", t, func() {
		tree := emptyTree()
		path := "R06.R00/R07.R01"

		Convey("When Ensure seeds a missing enter leaf", func() {
			tree.Ensure(path, ActionEnter)

			decision, ok := get(tree, path)

			Convey("Leaf uses ThresholdStart resistance", func() {
				So(ok, ShouldBeTrue)
				So(decision.Action, ShouldEqual, "enter")
				So(decision.Resistance, ShouldEqual, system.Cfg.Learning.ThresholdStart)
			})
		})

		Convey("When Ensure is called again with a different action", func() {
			tree.Ensure(path, ActionEnter)
			tree.Ensure(path, ActionExit)

			decision, ok := get(tree, path)

			Convey("First writer wins", func() {
				So(ok, ShouldBeTrue)
				So(decision.Action, ShouldEqual, "enter")
				So(decision.Resistance, ShouldEqual, system.Cfg.Learning.ThresholdStart)
			})
		})
	})
}

func TestDelete(t *testing.T) {
	Convey("Setup", t, func() {
		tree := emptyTree()
		path := "R06.R00/R07.R01"
		put(tree, path, leaf(ActionEnter, 0.5))

		Convey("When Delete removes the path", func() {
			tree.Delete(path)

			_, ok := get(tree, path)

			Convey("Leaf is gone", func() {
				So(ok, ShouldBeFalse)
			})
		})
	})
}

func TestLeafObject(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When object keys use action suffixes", func() {
			path, action := leafObject("model/paths/R01/M1/enter.bin", "model/paths/")

			Convey("leafObject splits path and action", func() {
				So(path, ShouldEqual, "R01/M1")
				So(action, ShouldEqual, "enter")
			})
		})

		Convey("When object keys use legacy flat layout", func() {
			path, action := leafObject("model/paths/R01/M1.bin", "model/paths/")

			Convey("leafObject keeps path without action", func() {
				So(path, ShouldEqual, "R01/M1")
				So(action, ShouldEqual, "")
			})
		})
	})
}
