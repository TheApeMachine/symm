package agent

import (
	"errors"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/learning/associative/model"
	"github.com/theapemachine/symm/nomagique/learning/associative/reward"
)

// choiceEnvironment exercises the agent protocol independently of any resource accounting.
type choiceEnvironment struct {
	actions  []string
	context  []uint64
	decision Decision[string]
	mark     reward.Mark
	err      error
}

func (environment *choiceEnvironment) Feasible(string) ([]string, []uint64, error) {
	return environment.actions, environment.context, nil
}
func (environment *choiceEnvironment) Execute(decision *Decision[string]) error {
	environment.decision = *decision
	return environment.err
}
func (environment *choiceEnvironment) Objective() (reward.Mark, error) {
	return environment.mark, environment.err
}

func newAgentFixture(t testing.TB) (*Agent[string], *choiceEnvironment) {
	t.Helper()
	environment := &choiceEnvironment{actions: []string{"wait", "work"}, context: []uint64{13}, mark: reward.Mark{At: time.Unix(100, 0), Version: 1, Value: 10}}
	member, err := New(environment, model.New[string, string](), false)
	if err != nil {
		t.Fatal(err)
	}
	return member, environment
}

func TestNew(t *testing.T) {
	Convey("An agent requires an environment and retained model", t, func() {
		member, environment := newAgentFixture(t)
		So(member.Model.Ordered, ShouldBeTrue)
		_, err := New[string](nil, member.Model, false)
		So(err, ShouldNotBeNil)
		_, err = New(environment, nil, false)
		So(err, ShouldNotBeNil)
	})
}

func TestAgentActivate(t *testing.T) {
	Convey("The agent selects through its existing model and preserves issue-time context", t, func() {
		member, environment := newAgentFixture(t)
		context := []uint64{11, 12}
		for range 2 {
			So(member.Model.Observe("task", []uint64{13, 11, 12}, "wait", -1, 1), ShouldBeNil)
			So(member.Model.Observe("task", []uint64{13, 11, 12}, "work", 2, 1), ShouldBeNil)
		}
		So(member.Activate("task", environment.mark.At, context, 1), ShouldBeNil)
		So(environment.decision.Action, ShouldEqual, "work")
		So(member.Selected.Mean, ShouldEqual, 2)
		context[0], environment.context[0] = 99, 98
		So(member.Last.Context, ShouldResemble, []uint64{13, 11, 12})
		So(member.Pending[member.Last.ID] == member.Last, ShouldBeTrue)

		Convey("Execution uncertainty remains pending and visible", func() {
			environment.err = errors.New("acceptance unknown")
			So(member.Activate("task", environment.mark.At, context, 1), ShouldNotBeNil)
			So(member.Err, ShouldNotBeNil)
			So(member.Pending[member.Last.ID], ShouldNotBeNil)
		})
	})
}

func TestAgentMeasure(t *testing.T) {
	Convey("Objective improvement and later delay provide separate successive weak rewards", t, func() {
		member, environment := newAgentFixture(t)
		So(member.Measure(), ShouldBeNil)
		So(member.Activate("task", environment.mark.At, []uint64{1}, 1), ShouldBeNil)
		environment.mark = reward.Mark{At: time.Unix(102, 0), Version: 2, Value: 14}
		So(member.Measure(), ShouldBeNil)
		So(member.Reward.Rate, ShouldEqual, 2)
		So(member.Reward.HasPriorRate, ShouldBeFalse)
		environment.mark = reward.Mark{At: time.Unix(104, 0), Version: 3, Value: 14}
		So(member.Measure(), ShouldBeNil)
		So(member.Reward.Differential, ShouldEqual, -4)
		reading := member.Model.Recall("task", member.Last.Context, member.Last.Action)
		So(reading.Mean, ShouldEqual, 0)
		So(reading.Samples, ShouldEqual, 2)
		So(reading.Provisional, ShouldBeTrue)
		So(member.Measure(), ShouldBeNil)
		So(member.Model.Recall("task", member.Last.Context, member.Last.Action), ShouldResemble, reading)

		Convey("Regressed marks do not rewrite the reward or evidence", func() {
			before := member.Reward
			environment.mark.Version--
			So(member.Measure(), ShouldNotBeNil)
			So(member.Reward, ShouldResemble, before)
			So(member.Model.Recall("task", member.Last.Context, member.Last.Action), ShouldResemble, reading)
		})
	})
}

func TestAgentResolve(t *testing.T) {
	Convey("Positive, negative and measured zero outcomes are retained exactly once", t, func() {
		member, environment := newAgentFixture(t)
		for _, value := range []float64{-2, 0, 3} {
			So(member.Activate("task", environment.mark.At, []uint64{1}, 1), ShouldBeNil)
			identity := member.Last.ID
			decision, err := member.Resolve(identity, value)
			So(err, ShouldBeNil)
			So(*decision.Outcome, ShouldEqual, value)
			So(member.Pending[identity], ShouldBeNil)
			_, err = member.Resolve(identity, value)
			So(err, ShouldNotBeNil)
		}
		So(member.Reading.Mean, ShouldAlmostEqual, 1.0/3)
		So(member.Reading.Samples, ShouldEqual, 3)
		So(member.Positive, ShouldEqual, 1)
		So(member.Negative, ShouldEqual, 1)
	})
}

func TestAgentAbort(t *testing.T) {
	Convey("An explicit unrealized decision supplies no completed outcome", t, func() {
		member, environment := newAgentFixture(t)
		So(member.Activate("task", environment.mark.At, []uint64{1}, 1), ShouldBeNil)
		So(member.Abort(member.Last.ID), ShouldBeNil)
		reading := member.Model.Recall("task", member.Last.Context, member.Last.Action)
		So(reading.Pending, ShouldEqual, 0)
		So(reading.Defined, ShouldBeFalse)
		So(member.Abort(member.Last.ID), ShouldNotBeNil)
	})
}
