package learning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestModelSelect(t *testing.T) {
	Convey("Given feasible numerical actions with distinct completed outcomes", t, func() {
		model := NewModel[string, int]()
		context := []uint64{3, 2, 1}
		actions := []int{0, 1, 2}

		for _, action := range actions {
			for range 3 {
				identity, err := model.Issue("first", context, action, 1)
				So(err, ShouldBeNil)
				_, err = model.Resolve(identity, float64(action)-1)
				So(err, ShouldBeNil)
			}
		}

		Convey("Selection uses the strongest evidenced mean", func() {
			for _, explore := range []bool{false, true} {
				action, prior, err := model.Select("first", context, actions, explore)
				So(err, ShouldBeNil)
				So(action, ShouldEqual, 2)
				So(prior.Samples, ShouldEqual, 3)
			}
		})

		Convey("A changed outcome regime changes the chosen action", func() {
			for range 6 {
				identity, err := model.Issue("first", context, 2, 1)
				So(err, ShouldBeNil)
				_, err = model.Resolve(identity, -3)
				So(err, ShouldBeNil)
			}
			action, _, err := model.Select("first", context, actions, false)
			So(err, ShouldBeNil)
			So(action, ShouldEqual, 1)
		})

		Convey("Parallel unknown selections account for already issued work", func() {
			selected := map[int]bool{}

			for range actions {
				action, prior, err := model.Select("second", context, actions, true)
				So(err, ShouldBeNil)
				So(prior.Defined, ShouldBeFalse)
				So(selected[action], ShouldBeFalse)
				selected[action] = true
				_, err = model.Issue("second", context, action, 1)
				So(err, ShouldBeNil)
			}
		})

		Convey("No feasible action produces a visible error", func() {
			_, _, err := model.Select("first", context, nil, true)
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkModelSelect(b *testing.B) {
	model := NewModel[string, int]()
	context, actions := []uint64{3, 2, 1}, []int{0, 1, 2, 3}

	for _, action := range actions {
		for _, value := range []float64{-1, 0, 2} {
			identity, err := model.Issue("first", context, action, 0.75)

			if err != nil {
				b.Fatal(err)
			}

			if _, err := model.Resolve(identity, value); err != nil {
				b.Fatal(err)
			}
		}
	}

	b.ReportAllocs()

	for b.Loop() {
		if _, _, err := model.Select("first", context, actions, true); err != nil {
			b.Fatal(err)
		}
	}
}
