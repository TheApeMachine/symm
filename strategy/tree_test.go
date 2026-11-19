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
			predicted, strength, matched := tree.Predict(tokensAt("R06.R00", 1.0))

			Convey("Predict returns ActionNone without a match", func() {
				So(matched, ShouldBeFalse)
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
					predicted, strength, matched := tree.Predict(tokensAt(prefix, 1.0))

					Convey("Predict returns ActionEnter", func() {
						So(matched, ShouldBeTrue)
						So(predicted, ShouldEqual, ActionEnter)
						So(strength, ShouldEqual, 0.75)
					})
				})

				Convey(fmt.Sprintf("When brightness for %s is below resistance", prefix), func() {
					predicted, strength, matched := tree.Predict(tokensAt(prefix, 0.5))

					Convey("Predict returns ActionNone with matched veto", func() {
						So(matched, ShouldBeTrue)
						So(predicted, ShouldEqual, ActionNone)
						So(strength, ShouldEqual, 0.75)
					})
				})
			}
		})

		Convey("When an exit extension sits under a hold path", func() {
			predicted, strength, matched := tree.Predict(
				tokensAt("R09.R01/R10.R02", 1.0),
			)

			Convey("Predict reads the hold leaf only", func() {
				So(matched, ShouldBeTrue)
				So(predicted, ShouldEqual, ActionHold)
				So(strength, ShouldEqual, 0.75)
				_, ok := get(tree, "R09.R01/R10.R02/R11.R03")
				So(ok, ShouldBeTrue)
			})
		})

		Convey("When a high-resistance leaf is the only match", func() {
			tree := emptyTree()
			path := "R06.R00/R07.R01"
			put(tree, path, leaf(ActionEnter, 0.85))

			predicted, strength, matched := tree.Predict(tokensAt(path, 1.0))

			Convey("Leaf is tombstoned at ceil and Predict returns matched ActionNone", func() {
				So(matched, ShouldBeTrue)
				So(predicted, ShouldEqual, ActionNone)
				So(strength, ShouldEqual, system.Cfg.Learning.ThresholdCeil)
				decision, ok := get(tree, path)
				So(ok, ShouldBeTrue)
				So(decision.Resistance, ShouldEqual, system.Cfg.Learning.ThresholdCeil)
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

			Convey("Resistance drops by the delta", func() {
				So(ok, ShouldBeTrue)
				So(decision.Resistance, ShouldEqual, start-0.05)
			})
		})

		Convey("When a positive delta is applied", func() {
			tree := emptyTree()
			put(tree, path, leaf(ActionEnter, start))
			tree.Update(path, 0.05)

			decision, ok := get(tree, path)

			Convey("Resistance rises by the delta", func() {
				So(ok, ShouldBeTrue)
				So(decision.Resistance, ShouldEqual, start+0.05)
			})
		})

		Convey("When an extension shares the path prefix", func() {
			tree := emptyTree()
			child := path + "/R08.R02"
			put(tree, path, leaf(ActionEnter, start))
			put(tree, child, leaf(ActionExit, start))
			tree.Update(path, 0.05)

			parent, okParent := get(tree, path)
			extension, okChild := get(tree, child)

			Convey("Only the exact leaf moves", func() {
				So(okParent, ShouldBeTrue)
				So(parent.Resistance, ShouldEqual, start+0.05)
				So(okChild, ShouldBeTrue)
				So(extension.Resistance, ShouldEqual, start)
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

func BenchmarkNewAction(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = NewAction("enter")
	}
}

func BenchmarkActionString(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = ActionEnter.String()
	}
}

func BenchmarkNewTree(b *testing.B) {
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		_ = NewTree(ctx, "model/paths/", nil)
	}
}

func BenchmarkPredict(b *testing.B) {
	tree := emptyTree()
	put(tree, "R06.R00/R07.R01", leaf(ActionEnter, 0.75))
	put(tree, "R06.R00/R07.R01/R08.R02", leaf(ActionEnter, 0.75))
	cut := tokensAt("R06.R00/R07.R01/R08.R02", 1.0)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		tree.Predict(cut)
	}
}

func BenchmarkUpdate(b *testing.B) {
	path := "R06.R00/R07.R01"
	tree := emptyTree()
	put(tree, path, leaf(ActionEnter, seedResistance()))

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		tree.Update(path, -0.05)
	}
}

func BenchmarkInsert(b *testing.B) {
	tree := emptyTree()
	path := "R06.R00/R07.R01"
	decision := leaf(ActionEnter, 0.42)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		tree.Insert(path, decision)
	}
}

func BenchmarkEnsure(b *testing.B) {
	tree := emptyTree()
	path := "R06.R00/R07.R01"

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		tree.Ensure(path, ActionEnter)
	}
}

func BenchmarkDelete(b *testing.B) {
	tree := emptyTree()
	path := "R06.R00/R07.R01"
	put(tree, path, leaf(ActionEnter, 0.5))

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		tree.Delete(path)
		put(tree, path, leaf(ActionEnter, 0.5))
	}
}

