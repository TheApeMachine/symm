package associative

import (
	"testing"
	"time"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/nomagique/transport"
)

/* encodeImpulse drives the context primitive with one impulse. */
func encodeImpulse(context core.Primitive, impulse grid.Impulse) ContextResult {
	evaluation := transport.NewEvaluate(context)
	var result ContextResult

	for out := range evaluation.Next(
		transport.NewOne(unsafe.Pointer(&ContextCommand{Encode: &impulse})).Next(nil),
	) {
		result = *(*ContextResult)(out)
	}

	return result
}

/* askAgent drives the agent primitive with one command. */
func askAgent(agent core.Primitive, command *Command) (Result, error) {
	evaluation := transport.NewEvaluate(agent)
	var result Result

	for out := range evaluation.Next(transport.NewOne(unsafe.Pointer(command)).Next(nil)) {
		result = *(*Result)(out)
	}

	return result, evaluation.Error()
}

func TestContextEncode(t *testing.T) {
	Convey("The temporal trajectory is framed as length-framed region tokens", t, func() {
		context := NewContext()
		at := time.Unix(1, 0)

		quiet := encodeImpulse(context, grid.Impulse{At: at, Ready: true})
		So(quiet.Sequence, ShouldBeNil)
		So(quiet.History, ShouldEqual, 0)

		first := encodeImpulse(context, grid.Impulse{
			At: at, Version: 1, Ready: true,
			Regions: []grid.Region{{Condition: 7}, {Condition: 3}},
		})
		So(first.History, ShouldEqual, 1)
		So(first.Sequence, ShouldResemble, []byte{
			0, 0, 0, 2,
			0, 0, 0, 0, 0, 0, 0, 3,
			0, 0, 0, 0, 0, 0, 0, 7,
		})

		Convey("a repeated version does not extend the history", func() {
			repeat := encodeImpulse(context, grid.Impulse{
				At: at, Version: 1, Ready: true,
				Regions: []grid.Region{{Condition: 9}},
			})
			So(repeat.History, ShouldEqual, 1)
			So(repeat.Sequence, ShouldResemble, first.Sequence)
		})

		Convey("a new version appends its own frame", func() {
			second := encodeImpulse(context, grid.Impulse{
				At: at, Version: 2, Ready: true,
				Regions: []grid.Region{{Condition: 5}},
			})
			So(second.History, ShouldEqual, 2)
			So(len(second.Sequence), ShouldEqual, len(first.Sequence)+4+8)
		})

		Convey("an unready impulse leaves the history intact", func() {
			third := encodeImpulse(context, grid.Impulse{At: at, Version: 3})
			So(third.History, ShouldEqual, 1)
			So(third.Sequence, ShouldNotBeNil)
		})

		Convey("reset clears observation-local history", func() {
			evaluation := transport.NewEvaluate(context)

			for range evaluation.Next(transport.NewOne(unsafe.Pointer(
				&ContextCommand{Reset: &ResetSignal{}},
			)).Next(nil)) {
			}

			So(evaluation.Error(), ShouldBeNil)
			cleared := encodeImpulse(context, grid.Impulse{At: at})
			So(cleared.Sequence, ShouldBeNil)
			So(cleared.History, ShouldEqual, 0)
		})
	})
}

func TestAgentLearnRecall(t *testing.T) {
	Convey("An agent learns what its regions precede and recalls it", t, func() {
		agent := NewAgent()
		at := time.Unix(1, 0)

		empty, err := askAgent(agent, &Command{Learn: &Learn{Impulse: grid.Impulse{At: at}}})
		So(err, ShouldBeNil)
		So(empty.Sequence, ShouldBeNil)

		learned, err := askAgent(agent, &Command{Learn: &Learn{
			Impulse: grid.Impulse{At: at, Version: 1, Ready: true,
				Regions: []grid.Region{{Condition: 7}}},
			Class: []byte("enter"),
		}})
		So(err, ShouldBeNil)
		So(len(learned.Sequence), ShouldEqual, 4+8)

		Convey("recall answers from the same sequence without writing", func() {
			recall, err := askAgent(agent, &Command{Recall: &Recall{
				Impulse: grid.Impulse{At: at, Version: 2, Ready: true,
					Regions: []grid.Region{{Condition: 7}}},
			}})
			So(err, ShouldBeNil)
			So(recall.Sequence, ShouldNotBeNil)
			So(recall.Evaluation.Context, ShouldNotBeNil)
		})

		Convey("reset clears the temporal history the agent recognises", func() {
			_, err := askAgent(agent, &Command{Reset: &ResetSignal{}})
			So(err, ShouldBeNil)

			fresh, err := askAgent(agent, &Command{Recall: &Recall{Impulse: grid.Impulse{At: at}}})
			So(err, ShouldBeNil)
			So(fresh.Sequence, ShouldBeNil)
			So(fresh.Evaluation.IsBreak, ShouldBeTrue)
		})
	})
}
