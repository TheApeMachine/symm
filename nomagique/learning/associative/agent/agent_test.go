package agent

import (
	"errors"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/nomagique/learning/associative/reward"
	"github.com/theapemachine/symm/nomagique/runtime"
)

// choiceEnvironment exercises the agent protocol independently of any resource accounting.
type choiceEnvironment struct {
	actions     []string
	context     []uint64
	decision    Decision[string]
	mark        reward.Mark
	err         error
	unavailable bool
}

func (environment *choiceEnvironment) Feasible(string) ([]string, []uint64, error) {
	return environment.actions, environment.context, nil
}
func (environment *choiceEnvironment) Execute(decision *Decision[string]) error {
	environment.decision = *decision
	return environment.err
}
func (environment *choiceEnvironment) Objective() (*reward.Mark, error) {
	if environment.unavailable {
		return nil, environment.err
	}

	return &environment.mark, environment.err
}

func newAgentFixture(t testing.TB) (*Agent[string], *choiceEnvironment) {
	t.Helper()
	environment := &choiceEnvironment{actions: []string{"wait", "work"}, context: []uint64{13}, mark: reward.Mark{At: time.Unix(100, 0), Version: 1, Value: 10}}
	member, err := New(t.Context(), environment, cognition.NewEngine(cognition.DefaultConfig()), false)
	if err != nil {
		t.Fatal(err)
	}
	return member, environment
}

func TestNew(t *testing.T) {
	Convey("An agent composes the runtime and existing cognition owner", t, func() {
		member, environment := newAgentFixture(t)
		So(member.System, ShouldNotBeNil)
		So(member.Status(), ShouldEqual, runtime.INIT)
		_, err := New[string](t.Context(), nil, member.Model, false)
		So(err, ShouldNotBeNil)
		_, err = New(t.Context(), environment, nil, false)
		So(err, ShouldNotBeNil)
	})
}

func TestAgentActivate(t *testing.T) {
	Convey("Live inference uses the cognitive winner under the original context", t, func() {
		member, environment := newAgentFixture(t)
		context := []uint64{11, 12}
		encoded := ContextKey("task", []uint64{13, 11, 12})
		for range 2 {
			member.Model.Observe(encoded, []byte("wait"), -1)
			member.Model.Observe(encoded, []byte("work"), 2)
		}
		So(member.Activate("task", environment.mark.At, context, 1), ShouldBeNil)
		So(environment.decision.Action, ShouldEqual, "work")
		So(member.Last.Evaluation.WinnerClass, ShouldEqual, "work")
		So(member.Status(), ShouldEqual, runtime.READY)
		context[0], environment.context[0], environment.actions[0] = 99, 98, "changed"
		So(member.Last.Context, ShouldResemble, []uint64{13, 11, 12})
		So(member.Last.Alternatives, ShouldResemble, []string{"wait", "work"})
		So(member.Pending[member.Last.ID], ShouldEqual, member.Last)

		Convey("Execution uncertainty remains pending and stops the runtime owner", func() {
			environment.err = errors.New("acceptance unknown")
			member.Explore = true
			So(member.Activate("task", environment.mark.At, context, 1), ShouldNotBeNil)
			So(member.Error(), ShouldNotBeNil)
			So(member.Status(), ShouldEqual, runtime.ERROR)
			So(member.Pending[member.Last.ID], ShouldNotBeNil)
		})
	})

	Convey("Missing or infeasible inference produces no artificial wait sample", t, func() {
		member, environment := newAgentFixture(t)
		So(member.Activate("task", environment.mark.At, []uint64{1}, 1), ShouldBeNil)
		So(member.Status(), ShouldEqual, runtime.WAITING)
		So(member.Pending, ShouldBeEmpty)
		member.Model.Observe(ContextKey("task", []uint64{13, 1}), []byte("unavailable action"))
		So(member.Activate("task", environment.mark.At, []uint64{1}, 1), ShouldBeNil)
		So(member.Decisions, ShouldEqual, 0)
	})
}

func TestAgentStep(t *testing.T) {
	Convey("Only ready regions reach the agent and breaks reset its active sequence", t, func() {
		member, environment := newAgentFixture(t)
		member.Explore = true
		impulse := grid.Impulse{Label: "task", At: environment.mark.At, From: environment.mark.At,
			Regions: []grid.Region{{Condition: 11, Strength: 1, Authority: 1}}}
		member.Step(impulse)
		So(member.Decisions, ShouldEqual, 0)
		So(member.Status(), ShouldEqual, runtime.WAITING)
		impulse.Ready = true
		member.Step(impulse)
		So(member.Decisions, ShouldEqual, 1)
		So(member.Last.Context, ShouldResemble, []uint64{13, uint64(1)<<63 | 2, 11})
		So(member.Last.Evaluation.IsBreak, ShouldBeTrue)
		So(member.histories["task"], ShouldBeEmpty)
		environment.err = errors.New("objective unavailable")
		member.Step(impulse)
		So(member.Error(), ShouldNotBeNil)
		So(member.Decisions, ShouldEqual, 1)
		member.Step(impulse)
		So(member.Decisions, ShouldEqual, 1)
	})
}

func TestAgentMeasure(t *testing.T) {
	Convey("Wallet marks and missing marks cannot train the policy", t, func() {
		member, environment := newAgentFixture(t)
		before := member.Model.Evaluate([]byte("task"))
		So(member.Measure(), ShouldBeNil)
		environment.mark = reward.Mark{At: time.Unix(102, 0), Version: 2, Value: 14}
		So(member.Measure(), ShouldBeNil)
		So(member.Reward.Rate, ShouldEqual, 2)
		environment.mark = reward.Mark{At: time.Unix(104, 0), Version: 3, Value: 14}
		So(member.Measure(), ShouldBeNil)
		So(member.Reward.Differential, ShouldEqual, -4)
		So(member.Model.Evaluate([]byte("task")), ShouldResemble, before)
		observed := member.Reward
		environment.unavailable = true
		So(member.Measure(), ShouldBeNil)
		So(member.Reward, ShouldResemble, observed)
		environment.unavailable = false
		environment.mark.Version--
		So(member.Measure(), ShouldNotBeNil)
		So(member.Reward, ShouldResemble, observed)
		So(member.Model.Evaluate([]byte("task")), ShouldResemble, before)
	})
}

func TestAgentResolve(t *testing.T) {
	Convey("Forward grades retain signed statistics without training the replay policy", t, func() {
		member, environment := newAgentFixture(t)
		encoded := ContextKey("task", []uint64{13, 1})
		member.Model.Observe(encoded, []byte("work"))
		before := member.Model.Evaluate(encoded)
		for _, value := range []float64{-2, 0, 3} {
			So(member.Activate("task", environment.mark.At, []uint64{1}, 1), ShouldBeNil)
			decision, err := member.Resolve(member.Last.ID, value)
			So(err, ShouldBeNil)
			So(*decision.Outcome, ShouldEqual, value)
			So(member.Pending[decision.ID], ShouldBeNil)
		}
		So(member.Reading.Mean, ShouldAlmostEqual, 1.0/3)
		So(member.Reading.Samples, ShouldEqual, 3)
		So(member.Positive, ShouldEqual, 1)
		So(member.Negative, ShouldEqual, 1)
		So(member.Model.Evaluate(encoded), ShouldResemble, before)
		_, err := member.Resolve(member.Last.ID, 3)
		So(err, ShouldNotBeNil)
		So(member.Reading.Samples, ShouldEqual, 3)
	})

	Convey("An explorer applies delayed feedback to the exact issued action", t, func() {
		member, environment := newAgentFixture(t)
		member.Explore = true
		So(member.Activate("task", environment.mark.At, []uint64{1}, 1), ShouldBeNil)
		encoded := ContextKey("task", member.Last.Context)
		So(member.Model.Evaluate(encoded).WinnerClass, ShouldBeEmpty)
		_, err := member.Resolve(member.Last.ID, -2)
		So(err, ShouldBeNil)
		So(member.Model.Evaluate(encoded).WinnerClass, ShouldEqual, member.Last.Action)
		So(member.Pending, ShouldBeEmpty)
		before := member.Model.Evaluate(encoded)
		_, err = member.Resolve(member.Last.ID, -2)
		So(err, ShouldNotBeNil)
		So(member.Model.Evaluate(encoded), ShouldResemble, before)
	})
}

func TestAgentAbort(t *testing.T) {
	Convey("An explicit unrealized operation supplies no learning", t, func() {
		member, environment := newAgentFixture(t)
		member.Explore = true
		So(member.Activate("task", environment.mark.At, []uint64{1}, 1), ShouldBeNil)
		So(member.Abort(member.Last.ID), ShouldBeNil)
		So(member.Pending, ShouldBeEmpty)
		So(member.Model.Evaluate(ContextKey("task", member.Last.Context)).WinnerClass, ShouldBeEmpty)
		So(member.Abort(member.Last.ID), ShouldNotBeNil)
	})
}

func BenchmarkAgentMeasure(b *testing.B) {
	member, environment := newAgentFixture(b)
	member.Explore = true
	b.ReportAllocs()

	for b.Loop() {
		environment.mark.Version++
		environment.mark.At = environment.mark.At.Add(time.Second)
		environment.unavailable = environment.mark.Version%2 == 0

		if err := member.Measure(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAgentActivate(b *testing.B) {
	for _, pendingCount := range []int{0, 10000} {
		b.Run(fmt.Sprint(pendingCount), func(b *testing.B) {
			member, environment := newAgentFixture(b)
			member.Explore = true
			for range pendingCount {
				if err := member.Activate("task", environment.mark.At, []uint64{1, 2}, 1); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportAllocs()
			for b.Loop() {
				if err := member.Activate("task", environment.mark.At, []uint64{1, 2}, 1); err != nil {
					b.Fatal(err)
				}
				if err := member.Abort(member.Last.ID); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
