package execution_test

import (
	"iter"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/execution"
)

func TestExecutionPipeline(t *testing.T) {
	Convey("Given execution zero-struct Value closures", t, func() {
		decide := execution.NewDecide()
		gate := execution.NewGate()
		submit := execution.NewSubmit()

		Convey("Nil evaluation returns wait", func() {
			action := decide(nil)
			So(action, ShouldEqual, "wait")
			allowed := gate(action)
			So(allowed, ShouldEqual, "wait")
			event := submit(allowed)
			So(event, ShouldBeNil)
		})

		Convey("Valid evaluation produces enter execution", func() {
			mockEval := cognition.Evaluation(func() (
				[]byte, []byte, float64, float64, uint64, float64, bool, float64, iter.Seq2[[]byte, float64],
			) {
				return []byte("enter"), nil, 0.9, 1.5, 10, 0.5, false, 0.1, nil
			})

			action := decide(mockEval)
			So(action, ShouldEqual, "enter")

			allowed := gate(action)
			So(allowed, ShouldEqual, "enter")

			event := submit(allowed)
			So(event, ShouldNotBeNil)
			So(event["action"], ShouldEqual, "enter")
			So(event["status"], ShouldEqual, "SUBMITTED")
		})
	})
}
