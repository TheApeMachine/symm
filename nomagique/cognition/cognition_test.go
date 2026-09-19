package cognition_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/cognition"
)

func TestCognitionPipeline(t *testing.T) {
	Convey("Given cognition zero-struct Value closures", t, func() {
		root, stepCounter := cognition.NewMemory()
		reinforce := cognition.NewReinforce(root, stepCounter)
		attractor := cognition.NewAttractor(root)
		classifier := cognition.NewClassification()

		Convey("An unseen context produces no evaluation", func() {
			candidates := attractor([]byte("unknown_context"))
			classResult := classifier(candidates)
			So(classResult, ShouldBeNil)
		})

		Convey("Reinforcing an association records empirical weight and evaluates winning class", func() {
			assoc := [2][]byte{[]byte("r0->r1"), []byte("enter")}
			
			res := reinforce(assoc)(1.0)
			So(res, ShouldNotBeNil)

			candidates := attractor([]byte("r0->r1"))
			classResult := classifier(candidates)

			So(classResult, ShouldNotBeNil)
			
			winner, _, confidence, _, support := classResult()
			So(string(winner), ShouldEqual, "enter")
			So(confidence, ShouldEqual, 1.0)
			So(support, ShouldEqual, 1)
		})

		Convey("Graded positive feedback strengthens the association", func() {
			assoc := [2][]byte{[]byte("r0->r1"), []byte("enter")}
			reinforce(assoc)(1.0) // initial observation
			reinforce(assoc)(2.5) // graded reinforcement

			candidates := attractor([]byte("r0->r1"))
			classResult := classifier(candidates)

			So(classResult, ShouldNotBeNil)
			winner, _, _, _, support := classResult()
			So(string(winner), ShouldEqual, "enter")
			So(support, ShouldEqual, 2)
		})

		Convey("Associate converts transitions into associations", func() {
			associate := cognition.NewAssociate()

			first := associate([]byte("r0"))
			So(first[0], ShouldNotBeNil)
			So(string(first[0]), ShouldEqual, "r0")
			So(first[1], ShouldBeNil)

			second := associate([]byte("r1"))
			So(second[0], ShouldNotBeNil)
			So(string(second[0]), ShouldEqual, "r0")
			So(string(second[1]), ShouldEqual, "r1")

			third := associate([]byte("r2"))
			So(third[0], ShouldNotBeNil)
			So(string(third[0]), ShouldEqual, "r1")
			So(string(third[1]), ShouldEqual, "r2")
		})
	})
}
