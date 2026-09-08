package associative

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative/agent"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/nomagique/learning/associative/reward"
)

// operation is a domain-owned action with parameters, not a package action enum.
type operation struct {
	Kind  string
	Units int
}

// resourceEnvironment assigns finite work capacity to independent task queues.
// This is the single fixture owner for the population's domain boundary.
type resourceEnvironment struct {
	available        int
	held             map[string]int
	choices          []operation
	mark             reward.Mark
	executed         []agent.Decision[operation]
	failure          error
	objectiveFailure error
	beforeObjective  func()
	beforeExecute    func()
}

func (environment *resourceEnvironment) Feasible(label string) ([]operation, []uint64, error) {
	actions := []operation{}
	for _, action := range environment.choices {
		if action.Kind == "allocate" && action.Units > environment.available {
			continue
		}
		if action.Kind == "release" && action.Units > environment.held[label] {
			continue
		}
		actions = append(actions, action)
	}
	// The high bit separates fixture resource-state tokens from grid conditions.
	return actions, []uint64{1<<63 | uint64(environment.held[label])}, nil
}

func (environment *resourceEnvironment) Execute(decision *agent.Decision[operation]) error {
	if environment.beforeExecute != nil {
		environment.beforeExecute()
	}

	if environment.failure != nil {
		return environment.failure
	}
	switch decision.Action.Kind {
	case "allocate":
		environment.available -= decision.Action.Units
		environment.held[decision.Label] += decision.Action.Units
	case "release":
		environment.available += decision.Action.Units
		environment.held[decision.Label] -= decision.Action.Units
	}
	environment.executed = append(environment.executed, *decision)
	return nil
}

func (environment *resourceEnvironment) Objective() (*reward.Mark, error) {
	if environment.beforeObjective != nil {
		environment.beforeObjective()
	}

	return &environment.mark, environment.objectiveFailure
}

type memoryCheckpoint struct {
	data    []byte
	err     error
	entered chan struct{}
	release chan struct{}
}

func (checkpoint *memoryCheckpoint) Load(context.Context) ([]byte, bool, error) {
	return checkpoint.data, checkpoint.data != nil, checkpoint.err
}
func (checkpoint *memoryCheckpoint) Save(_ context.Context, data []byte) error {
	if checkpoint.err != nil {
		return checkpoint.err
	}

	if checkpoint.entered != nil {
		close(checkpoint.entered)
		<-checkpoint.release
	}
	checkpoint.data = slices.Clone(data)
	return nil
}

func newPopulationFixture(t testing.TB) (*Population[operation], []*resourceEnvironment) {
	t.Helper()
	environments := []*resourceEnvironment{}
	boundaries := []agent.Environment[operation]{}
	// Three independent resource pools: one evaluator and two explorers.
	for range 3 {
		environment := &resourceEnvironment{available: 8, held: map[string]int{}, choices: []operation{{"allocate", 1}, {"idle", 0}}, mark: reward.Mark{At: time.Unix(100, 0), Version: 1, Value: 8}}
		environments = append(environments, environment)
		boundaries = append(boundaries, environment)
	}
	population, err := NewPopulation(boundaries...)
	if err != nil {
		t.Fatal(err)
	}
	return population, environments
}

func observationFixture(sequence int, label string) agent.Observation {
	at := time.Unix(100+int64(sequence), 0)
	value := float64(sequence%4 - 2)
	measurement := data.NewMeasurement[float64]("", label, "sensor", at, at)
	measurement.PutMetric(data.Metric[float64]{Label: "load", Raw: value})
	measurement.PutMetric(data.Metric[float64]{Label: "pressure", Raw: -value})
	// Explicit prior regimes of this fixture, oldest first, supplied by its producer.
	return agent.Observation{At: at, Measurements: []*data.Measurement[float64]{measurement}, History: []uint64{700, 701}}
}

func drivePopulation(t testing.TB, population *Population[operation], environments []*resourceEnvironment) {
	t.Helper()
	// Rising, falling and reversing load on two independent task queues.
	for sequence := 1; sequence <= 24; sequence++ {
		label := []string{"queue-a", "queue-b"}[sequence%2]
		observation := observationFixture(sequence, label)
		for _, environment := range environments {
			environment.mark = reward.Mark{At: observation.At, Version: uint64(sequence + 1), Value: float64(8 - sequence%3)}
		}
		if err := population.Step(observation); err != nil {
			t.Fatal(err)
		}
	}
}

func TestNewPopulation(t *testing.T) {
	Convey("A population requires real environments and creates independent learners", t, func() {
		_, err := NewPopulation[operation]()
		So(err, ShouldNotBeNil)
		_, err = NewPopulation[operation](nil)
		So(err, ShouldNotBeNil)
		population, _ := newPopulationFixture(t)
		So(population.Agents[0].Explore, ShouldBeFalse)
		So(population.Agents[1].Explore, ShouldBeTrue)
		So(population.Agents[0].Model == population.Agents[1].Model, ShouldBeFalse)
	})
}

func TestPopulationStep(t *testing.T) {
	testPopulationPhases(t)

	Convey("Grid activity drives domain-owned finite actions for every agent", t, func() {
		population, environments := newPopulationFixture(t)
		drivePopulation(t, population, environments)
		So(population.Grid.Rows, ShouldHaveLength, 2)
		So(population.Grid.Version, ShouldEqual, 24)
		for index, member := range population.Agents {
			So(member.Decisions, ShouldBeGreaterThan, 0)
			So(member.Decisions, ShouldEqual, population.Agents[0].Decisions)
			So(environments[index].available, ShouldBeGreaterThanOrEqualTo, 0)
			So(member.Last.Context[1:3], ShouldResemble, []uint64{700, 701})
			So(member.Reward.TotalElapsed, ShouldBeGreaterThan, 0)
		}
		So(environments[0].held["queue-a"], ShouldBeGreaterThan, 0)
		So(environments[0].held["queue-b"], ShouldBeGreaterThan, 0)

		Convey("Missing observations do not issue another decision", func() {
			before := population.Agents[0].Decisions
			So(population.Step(agent.Observation{At: time.Unix(125, 0)}), ShouldBeNil)
			So(population.Agents[0].Decisions, ShouldEqual, before)
		})

		Convey("No feasible action is an explicit error", func() {
			environments[0].choices = nil
			So(population.Step(observationFixture(25, "queue-a")), ShouldNotBeNil)
			So(population.Agents[0].Err, ShouldNotBeNil)
		})

		Convey("Uncertain execution retains its issued identity until explicit abort", func() {
			environments[0].failure = errors.New("acceptance unknown")
			So(population.Step(observationFixture(25, "queue-a")), ShouldNotBeNil)
			member := population.Agents[0]
			identity := member.Last.ID
			So(member.Pending[identity], ShouldNotBeNil)
			So(member.Abort(identity), ShouldBeNil)
			So(member.Pending[identity], ShouldBeNil)
			So(member.Abort(identity), ShouldNotBeNil)
		})

		Convey("Objective failure is observable and does not invent a reward", func() {
			previous := population.Agents[0].Reward
			environments[0].objectiveFailure = errors.New("objective unavailable")
			So(population.Step(observationFixture(25, "queue-a")), ShouldNotBeNil)
			So(population.Agents[0].Reward, ShouldResemble, previous)
		})

		Convey("Objective and elapsed-time feedback train once per new mark", func() {
			member := population.Agents[0]
			last := member.Last
			before := member.Model.Recall(last.Label, nil, last.Action).Samples
			So(member.Measure(), ShouldBeNil)
			So(member.Model.Recall(last.Label, nil, last.Action).Samples, ShouldEqual, before)
			previousRate := member.Reward.Rate
			environments[0].mark.Version++
			environments[0].mark.At = environments[0].mark.At.Add(3 * time.Second)
			So(member.Measure(), ShouldBeNil)
			So(member.Reward.Reward, ShouldEqual, 0)
			So(member.Reward.Differential, ShouldAlmostEqual, -3*previousRate)
			So(member.Model.Recall(last.Label, nil, last.Action).Samples, ShouldEqual, before+1)
		})

		Convey("Release is selected through the same domain action boundary", func() {
			environments[0].choices = []operation{{"release", 1}}
			before := environments[0].held["queue-a"]
			So(population.Step(observationFixture(25, "queue-a")), ShouldBeNil)
			So(environments[0].held["queue-a"], ShouldEqual, before-1)
		})
	})
}

// These subtests block at the real Environment boundary, without timing sleeps.
func testPopulationPhases(t *testing.T) {
	for _, failure := range []string{"none", "measure", "execute"} {
		t.Run(failure, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				Convey("Independent agents overlap and every phase is joined", t, func() {
					population, environments := newPopulationFixture(t)
					drivePopulation(t, population, environments)
					measured := make(chan struct{}, len(environments))
					executed := make(chan struct{}, len(environments))
					measureRelease, executeRelease := make(chan struct{}), make(chan struct{})

					for _, environment := range environments {
						environment.beforeObjective = func() { measured <- struct{}{}; <-measureRelease }
						environment.beforeExecute = func() { executed <- struct{}{}; <-executeRelease }
					}

					if failure == "measure" {
						environments[0].objectiveFailure = errors.New("objective failed")
					}

					if failure == "execute" {
						environments[0].failure = errors.New("execution failed")
					}
					done := make(chan error, 1)
					go func() { done <- population.Step(observationFixture(25, "queue-a")) }()
					synctest.Wait()
					// Release barriers even if a regression makes the overlap assertion fail.
					measuring, premature := len(measured), len(executed)
					close(measureRelease)
					synctest.Wait()
					acting, returned := len(executed), len(done)
					close(executeRelease)
					err := <-done
					So(measuring, ShouldEqual, len(environments))
					So(premature, ShouldEqual, 0)

					if failure == "measure" {
						So(acting, ShouldEqual, 0)
						So(err, ShouldNotBeNil)
						return
					}
					So(acting, ShouldEqual, len(environments))
					So(returned, ShouldEqual, 0)

					if failure == "execute" {
						So(err, ShouldNotBeNil)
						So(population.Agents[0].Err, ShouldNotBeNil)
						So(population.Agents[1].Err, ShouldBeNil)
						return
					}
					So(err, ShouldBeNil)
				})
			})
		})
	}
}

func TestPopulationSave(t *testing.T) {
	Convey("Slow durable writes do not own the learning state lock", t, func() {
		population, environments := newPopulationFixture(t)
		drivePopulation(t, population, environments)
		checkpoint := &memoryCheckpoint{entered: make(chan struct{}), release: make(chan struct{})}
		saved := make(chan error, 1)
		go func() { saved <- population.Save(t.Context(), checkpoint) }()
		<-checkpoint.entered
		stepped := make(chan error, 1)
		go func() { stepped <- population.Step(observationFixture(25, "queue-a")) }()

		// A test synchronization deadline, not an observation or decision horizon.
		select {
		case err := <-stepped:
			close(checkpoint.release)
			So(err, ShouldBeNil)
		case <-time.After(time.Second):
			close(checkpoint.release)
			t.Fatal("durable write blocked the learning state")
		}
		So(<-saved, ShouldBeNil)
	})
}

func TestPopulationResolve(t *testing.T) {
	Convey("Delayed signed outcomes train their original decisions", t, func() {
		population, environments := newPopulationFixture(t)
		drivePopulation(t, population, environments)
		for index, value := range []float64{-4, 5, -2} {
			member := population.Agents[index]
			decision := member.Last
			reading := population.Agents[0].Model.Recall(decision.Label, nil, decision.Action)
			before := reading.Samples

			if reading.Provisional {
				before = 0
			}
			So(population.Resolve(index, decision.ID, value), ShouldBeNil)
			So(member.Reading.Mean, ShouldEqual, value)
			So(*decision.Outcome, ShouldEqual, value)
			So(member.Model.Recall(decision.Label, decision.Context, decision.Action).Provisional, ShouldBeFalse)
			So(population.Resolve(index, decision.ID, value), ShouldNotBeNil)
			reading = population.Agents[0].Model.Recall(decision.Label, nil, decision.Action)
			after := reading.Samples

			if reading.Provisional {
				after = 0
			}
			if index == 0 || value > 0 {
				So(after, ShouldEqual, before+1)
			}
			if index != 0 && value < 0 {
				So(after, ShouldEqual, before)
			}
		}
		So(population.Agents[0].Negative, ShouldEqual, 1)
		So(population.Agents[1].Positive, ShouldEqual, 1)
		So(population.Resolve(3, 1, 1), ShouldNotBeNil)
	})
}

func TestPopulationRestore(t *testing.T) {
	Convey("Only the consolidated model is persisted and restored into fresh agents", t, func() {
		population, environments := newPopulationFixture(t)
		drivePopulation(t, population, environments)
		decision := population.Agents[1].Last
		So(population.Resolve(1, decision.ID, 7), ShouldBeNil)
		checkpoint := &memoryCheckpoint{}
		So(population.Save(t.Context(), checkpoint), ShouldBeNil)
		restored, _ := newPopulationFixture(t)
		found, err := restored.Restore(t.Context(), checkpoint)
		So(err, ShouldBeNil)
		So(found, ShouldBeTrue)
		So(restored.Grid.Columns, ShouldResemble, population.Grid.Columns)
		for _, member := range restored.Agents {
			reading := member.Model.Recall(decision.Label, decision.Context, decision.Action)
			expected := population.Agents[0].Model.Recall(decision.Label, decision.Context, decision.Action)
			So(reading.Mean, ShouldEqual, expected.Mean)
			So(reading.Pending, ShouldEqual, 0)
			So(member.Pending, ShouldBeEmpty)
		}
		So(restored.Agents[0].Model == restored.Agents[1].Model, ShouldBeFalse)

		Convey("Learning continues after restart", func() {
			boundaries := []*resourceEnvironment{}
			for _, member := range restored.Agents {
				boundaries = append(boundaries, member.Environment.(*resourceEnvironment))
			}
			drivePopulation(t, restored, boundaries)
			So(restored.Agents[0].Decisions, ShouldBeGreaterThan, 0)
			_, err = restored.Restore(t.Context(), checkpoint)
			So(err, ShouldNotBeNil)
		})

		Convey("Missing, malformed and failed storage remain distinct", func() {
			fresh, _ := newPopulationFixture(t)
			found, err = fresh.Restore(t.Context(), &memoryCheckpoint{})
			So(err, ShouldBeNil)
			So(found, ShouldBeFalse)
			found, err = fresh.Restore(t.Context(), &memoryCheckpoint{data: []byte("invalid")})
			So(err, ShouldNotBeNil)
			So(found, ShouldBeFalse)
			failed := &memoryCheckpoint{err: errors.New("storage failed")}
			_, err = fresh.Restore(t.Context(), failed)
			So(err, ShouldNotBeNil)
			So(fresh.Save(t.Context(), failed), ShouldNotBeNil)
		})

		Convey("Conflicting feature identities cannot silently remap learned contexts", func() {
			invalid, _ := newPopulationFixture(t)
			invalid.Grid.Columns = [][2]string{{"sensor", "load"}, {"sensor", "load"}}
			So(invalid.Save(t.Context(), checkpoint), ShouldBeNil)
			fresh, _ := newPopulationFixture(t)
			found, err := fresh.Restore(t.Context(), checkpoint)
			So(err, ShouldNotBeNil)
			So(found, ShouldBeFalse)
			So(fresh.Grid.Columns, ShouldBeEmpty)
		})
	})
}

func BenchmarkPopulationStep(b *testing.B) {
	population, environments := newPopulationFixture(b)
	drivePopulation(b, population, environments)
	sequence := 24
	b.ReportAllocs()
	for b.Loop() {
		sequence++
		observation := observationFixture(sequence, fmt.Sprintf("queue-%d", sequence%2))
		for _, environment := range environments {
			environment.mark = reward.Mark{At: observation.At, Version: uint64(sequence + 1), Value: float64(8 - sequence%3)}
		}
		if err := population.Step(observation); err != nil {
			b.Fatal(err)
		}
		for index, member := range population.Agents {
			if member.Last == nil || member.Last.Outcome != nil {
				continue
			}
			if err := population.Resolve(index, member.Last.ID, float64(sequence%3-1)); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkPopulationSave(b *testing.B) {
	population, environments := newPopulationFixture(b)
	drivePopulation(b, population, environments)
	checkpoint := &memoryCheckpoint{}
	b.ReportAllocs()
	for b.Loop() {
		if err := population.Save(b.Context(), checkpoint); err != nil {
			b.Fatal(err)
		}
	}
}

func TestPopulationAbort(t *testing.T) {
	Convey("A decision with no alternative is released, not trained", t, func() {
		population, environments := newPopulationFixture(t)
		drivePopulation(t, population, environments)
		member := population.Agents[1]
		decision := member.Last
		So(decision, ShouldNotBeNil)
		before := member.Model.Recall(decision.Label, decision.Context, decision.Action)

		So(population.Abort(-1, decision.ID), ShouldNotBeNil)
		So(population.Abort(len(population.Agents), decision.ID), ShouldNotBeNil)
		So(population.Abort(1, decision.ID), ShouldBeNil)

		Convey("Its evidence is unchanged and it cannot be resolved afterwards", func() {
			after := member.Model.Recall(decision.Label, decision.Context, decision.Action)
			So(after.Samples, ShouldEqual, before.Samples)
			So(after.Mean, ShouldEqual, before.Mean)
			So(member.Pending, ShouldNotContainKey, decision.ID)
			So(population.Resolve(1, decision.ID, 1), ShouldNotBeNil)
			So(population.Abort(1, decision.ID), ShouldNotBeNil)
		})
	})
}

func TestPopulationLearn(t *testing.T) {
	Convey("Worker quantity identities are remapped before teaching the live model", t, func() {
		population, _ := newPopulationFixture(t)
		population.Grid.Column("live", "unrelated")
		columns := [][2]string{{"tape", "ask"}}
		marker := uint64(1)<<63 | 2
		context := []uint64{marker, grid.ConditionToken(1, 1, -1)}
		action := operation{Kind: "allocate"}
		So(population.Learn("task", columns, context, action, -0.2, 0.5), ShouldBeNil)
		So(population.Grid.Columns[1], ShouldResemble, columns[0])
		So(context[1], ShouldEqual, grid.ConditionToken(1, 1, -1))
		mapped := []uint64{marker, grid.ConditionToken(2, 1, -1)}
		reading := population.Agents[0].Model.Recall("task", mapped, action)
		So(reading.Mean, ShouldEqual, -0.2)
		So(reading.Depth, ShouldEqual, 2)

		Convey("An identity outside the worker dictionary is rejected", func() {
			So(population.Learn("task", nil, context, action, 1, 1), ShouldNotBeNil)
		})
	})
}

func BenchmarkPopulationLearn(b *testing.B) {
	population, _ := newPopulationFixture(b)
	columns := [][2]string{{"tape", "bid"}, {"tape", "ask"}}
	context := []uint64{1<<63 | 2, grid.ConditionToken(1, 1, -1), grid.ConditionToken(2, -1, 1)}
	action := operation{Kind: "allocate"}
	b.ReportAllocs()

	for b.Loop() {
		if err := population.Learn("task", columns, context, action, -0.2, 0.5); err != nil {
			b.Fatal(err)
		}
	}
}
