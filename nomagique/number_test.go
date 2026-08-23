package nomagique

import (
	"errors"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

var (
	numberDelta = types.MustIntern("test/number/delta")
	numberTotal = types.MustIntern("test/number/total")
	numberReady = types.MustIntern("test/number/ready")
)

func numberAccumulator(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
	delta, found := input.Get(numberDelta)
	if !found {
		return state, types.Frame{}, errors.New("missing delta")
	}
	if delta < 0 {
		state.Put(numberTotal, 999)
		return state, types.Frame{}, errors.New("negative delta")
	}
	total, _ := state.Get(numberTotal)
	total += delta
	state.Put(numberTotal, total)
	output := input
	output.Put(numberTotal, total)
	return state, output, nil
}

func TestNumberIsolationAndTransactions(t *testing.T) {
	Convey("Number owns isolated committed state for every key", t, func() {
		number := NewNumber[string](numberAccumulator)
		_, err := number.Step("left", types.Frame{}.Set(numberDelta, 2))
		So(err, ShouldBeNil)
		_, err = number.Step("left", types.Frame{}.Set(numberDelta, 3))
		So(err, ShouldBeNil)
		_, err = number.Step("right", types.Frame{}.Set(numberDelta, 7))
		So(err, ShouldBeNil)

		left, found := number.Project("left")
		So(found, ShouldBeTrue)
		So(left.MustGet(numberTotal), ShouldEqual, 5.0)
		right, found := number.Project("right")
		So(found, ShouldBeTrue)
		So(right.MustGet(numberTotal), ShouldEqual, 7.0)
	})

	Convey("A rejected keyed transition cannot poison committed state or output", t, func() {
		number := NewNumber[string](numberAccumulator)
		good, err := number.Step("key", types.Frame{}.Set(numberDelta, 4))
		So(err, ShouldBeNil)
		failed, err := number.Step("key", types.Frame{}.Set(numberDelta, -1))
		So(err, ShouldNotBeNil)
		So(failed.Equal(good), ShouldBeTrue)
		state, found := number.Project("key")
		So(found, ShouldBeTrue)
		So(state.MustGet(numberTotal), ShouldEqual, 4.0)
		last, found := number.Output("key")
		So(found, ShouldBeTrue)
		So(last.Equal(good), ShouldBeTrue)
		lastErr, found := number.Error("key")
		So(found, ShouldBeTrue)
		So(lastErr, ShouldNotBeNil)
	})

	Convey("Initial state is evaluated exactly once for a newly stored key", t, func() {
		calls := 0
		number := NewNumberWithInitial[string](func(key string) types.Frame {
			calls++
			return types.Frame{}.Set(numberTotal, float64(len(key)))
		}, numberAccumulator)
		_, err := number.Step("abcd", types.Frame{}.Set(numberDelta, 1))
		So(err, ShouldBeNil)
		_, err = number.Step("abcd", types.Frame{}.Set(numberDelta, 1))
		So(err, ShouldBeNil)
		state, _ := number.Project("abcd")
		So(state.MustGet(numberTotal), ShouldEqual, 6.0)
		So(calls, ShouldEqual, 1)
	})

	Convey("Range yields copies that cannot mutate owned state", t, func() {
		number := NewNumber[string](numberAccumulator)
		_, _ = number.Step("a", types.Frame{}.Set(numberDelta, 1))
		_, _ = number.Step("b", types.Frame{}.Set(numberDelta, 2))
		count := 0
		total := 0.0
		number.Range(func(_ string, state types.Frame) bool {
			count++
			total += state.MustGet(numberTotal)
			state.Put(numberTotal, 1000)
			return true
		})
		So(count, ShouldEqual, 2)
		So(total, ShouldEqual, 3.0)
		state, _ := number.Project("a")
		So(state.MustGet(numberTotal), ShouldEqual, 1.0)
	})

	Convey("Reset and Delete have explicit lifecycle semantics", t, func() {
		number := NewNumber[string](numberAccumulator)
		_, _ = number.Step("key", types.Frame{}.Set(numberDelta, 3))
		So(number.Reset("key", types.Frame{}.Set(numberTotal, 10)), ShouldBeNil)
		state, found := number.Project("key")
		So(found, ShouldBeTrue)
		So(state.MustGet(numberTotal), ShouldEqual, 10.0)
		_, found = number.Output("key")
		So(found, ShouldBeTrue)
		number.Delete("key")
		_, found = number.Project("key")
		So(found, ShouldBeFalse)
	})
}

func TestNumberConcurrency(t *testing.T) {
	Convey("Concurrent writers for one key are serialized without lost updates", t, func() {
		number := NewNumber[string](numberAccumulator)
		const workers = 16
		const iterations = 50
		failures := make(chan error, workers)
		var wait sync.WaitGroup
		for range workers {
			wait.Add(1)
			go func() {
				defer wait.Done()
				for range iterations {
					if _, err := number.Step("shared", types.Frame{}.Set(numberDelta, 1)); err != nil {
						failures <- err
						return
					}
				}
			}()
		}
		wait.Wait()
		close(failures)
		So(len(failures), ShouldEqual, 0)
		state, found := number.Project("shared")
		So(found, ShouldBeTrue)
		So(state.MustGet(numberTotal), ShouldEqual, float64(workers*iterations))
	})

	Convey("Different keys progress independently under concurrency", t, func() {
		number := NewNumber[int](numberAccumulator)
		const keys = 8
		const iterations = 40
		var wait sync.WaitGroup
		for key := range keys {
			wait.Add(1)
			go func(key int) {
				defer wait.Done()
				for range iterations {
					_, _ = number.Step(key, types.Frame{}.Set(numberDelta, 1))
				}
			}(key)
		}
		wait.Wait()
		for key := range keys {
			state, found := number.Project(key)
			So(found, ShouldBeTrue)
			So(state.MustGet(numberTotal), ShouldEqual, float64(iterations))
		}
	})

	Convey("Established-key Number steps do not allocate Frame snapshots", t, func() {
		number := NewNumber[string](numberAccumulator)
		input := types.Frame{}.Set(numberDelta, 1)
		_, err := number.Step("steady", input)
		So(err, ShouldBeNil)
		allocations := testing.AllocsPerRun(1000, func() {
			if _, stepErr := number.Step("steady", input); stepErr != nil {
				panic(stepErr)
			}
		})
		So(allocations, ShouldEqual, 0.0)
	})
}

func TestNumberCrossSectionAndSelection(t *testing.T) {
	pair := func(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
		output := types.Frame{}.Set(numberTotal, state.MustGet(numberTotal)+input.MustGet(numberTotal))
		return state, output, nil
	}
	reduce := func(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
		total, _ := state.Get(numberTotal)
		total += input.MustGet(numberTotal)
		state.Put(numberTotal, total)
		return state, state, nil
	}
	score := func(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
		output := input
		output.Put(numberReady, 1)
		return state, output, nil
	}

	Convey("CrossSection folds peers without mutating keyed state", t, func() {
		number := NewNumber[string](numberAccumulator)
		for key, value := range map[string]float64{"focal": 2, "first": 3, "second": 5} {
			_, _ = number.Step(key, types.Frame{}.Set(numberDelta, value))
		}
		output, ready, err := number.CrossSection("focal", pair, reduce, types.Identity)
		So(err, ShouldBeNil)
		So(ready, ShouldBeTrue)
		So(output.MustGet(numberTotal), ShouldEqual, 12.0)
		focal, _ := number.Project("focal")
		So(focal.MustGet(numberTotal), ShouldEqual, 2.0)
	})

	Convey("ArgMax requires a unique maximum above the exact median", t, func() {
		number := NewNumber[string](numberAccumulator)
		for key, value := range map[string]float64{"low": 1, "middle": 2, "leader": 5} {
			_, _ = number.Step(key, types.Frame{}.Set(numberDelta, value))
		}
		selected, maximum, median, ready, err := number.ArgMax(score, numberTotal, numberReady)
		So(err, ShouldBeNil)
		So(ready, ShouldBeTrue)
		So(selected, ShouldEqual, "leader")
		So(maximum, ShouldEqual, 5.0)
		So(median, ShouldEqual, 2.0)
	})

	Convey("A tied maximum is deliberately not selected", t, func() {
		number := NewNumber[string](numberAccumulator)
		for key, value := range map[string]float64{"a": 5, "b": 5, "c": 1} {
			_, _ = number.Step(key, types.Frame{}.Set(numberDelta, value))
		}
		_, _, _, ready, err := number.ArgMax(score, numberTotal, numberReady)
		So(err, ShouldBeNil)
		So(ready, ShouldBeFalse)
	})
}

func BenchmarkNumberEstablishedKey(b *testing.B) {
	number := NewNumber[string](numberAccumulator)
	input := types.Frame{}.Set(numberDelta, 1)
	_, _ = number.Step("symbol", input)
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		_, _ = number.Step("symbol", input)
	}
}
