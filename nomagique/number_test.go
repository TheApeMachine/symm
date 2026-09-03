package nomagique

import (
	"errors"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

var (
	numberDelta   = types.MustIntern("test/number/delta")
	numberTotal   = types.MustIntern("test/number/total")
	numberReady   = types.MustIntern("test/number/ready")
	numberPairSum = types.MustIntern("test/number/pair_sum")
)

func numberAccumulator(input *types.Frame) {
	delta, found := input.Get(numberDelta)

	if !found {
		input.Err = errors.New("missing delta")
	}

	if delta < 0 {
		input.Put(numberTotal, 999)
		input.Err = errors.New("negative delta")
	}

	total, _ := input.Get(numberTotal)
	total += delta
	input.Put(numberTotal, total)
}

func TestNumberIsolationAndTransactions(t *testing.T) {
	Convey("Number owns isolated committed state for every key", t, func() {
		number := NewNumber[string](numberAccumulator)
		output := number.Step("left", types.Frame{}.Set(numberDelta, 2))
		So(output.Err, ShouldBeNil)
		output = number.Step("left", types.Frame{}.Set(numberDelta, 3))
		So(output.Err, ShouldBeNil)
		output = number.Step("right", types.Frame{}.Set(numberDelta, 7))
		So(output.Err, ShouldBeNil)

		left, found := number.Project("left")
		So(found, ShouldBeTrue)
		So(left.MustGet(numberTotal), ShouldEqual, 5.0)
		right, found := number.Project("right")
		So(found, ShouldBeTrue)
		So(right.MustGet(numberTotal), ShouldEqual, 7.0)
	})

	Convey("A rejected keyed transition cannot poison committed state or output", t, func() {
		number := NewNumber[string](numberAccumulator)
		good := number.Step("key", types.Frame{}.Set(numberDelta, 4))
		So(good.Err, ShouldBeNil)
		failed := number.Step("key", types.Frame{}.Set(numberDelta, -1))
		So(failed.Err, ShouldNotBeNil)
		state, found := number.Project("key")
		So(found, ShouldBeTrue)
		So(state.MustGet(numberTotal), ShouldEqual, 4.0)
		last, found := number.Output("key")
		So(found, ShouldBeTrue)
		So(last.Equal(&good), ShouldBeTrue)
	})

	Convey("Initial state is evaluated exactly once for a newly stored key", t, func() {
		calls := 0
		number := NewNumberWithInitial(func(key string) types.Frame {
			calls++
			return types.Frame{}.Set(numberTotal, float64(len(key)))
		}, numberAccumulator)
		output := number.Step("abcd", types.Frame{}.Set(numberDelta, 1))
		So(output.Err, ShouldBeNil)
		output = number.Step("abcd", types.Frame{}.Set(numberDelta, 1))
		So(output.Err, ShouldBeNil)
		state, _ := number.Project("abcd")
		So(state.MustGet(numberTotal), ShouldEqual, 6.0)
		So(calls, ShouldEqual, 1)
	})

	Convey("Range yields copies that cannot mutate owned state", t, func() {
		number := NewNumber[string](numberAccumulator)
		_ = number.Step("a", types.Frame{}.Set(numberDelta, 1))
		_ = number.Step("b", types.Frame{}.Set(numberDelta, 2))
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
		_ = number.Step("key", types.Frame{}.Set(numberDelta, 3))
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
					if output := number.Step("shared", types.Frame{}.Set(numberDelta, 1)); output.Err != nil {
						failures <- output.Err
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
					_ = number.Step(key, types.Frame{}.Set(numberDelta, 1))
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
		output := number.Step("steady", input)
		So(output.Err, ShouldBeNil)
		// See TestSingleLifecycle: the returned frame is intentionally left
		// unbound so this measures Step's own steady-state churn rather than
		// the caller's unavoidable MaxSlots-wide return copy.
		allocations := testing.AllocsPerRun(1000, func() {
			number.Step("steady", input)
		})
		So(allocations, ShouldEqual, 0.0)
	})
}

func TestNumberCrossSectionAndSelection(t *testing.T) {
	pair := func(focal *types.Frame, peer *types.Frame) types.Frame {
		return types.Frame{}.Set(
			numberPairSum,
			focal.MustGet(numberTotal)+peer.MustGet(numberTotal),
		)
	}
	reduce := func(input *types.Frame) {
		total, _ := input.Get(numberTotal)
		total += input.MustGet(numberPairSum)
		input.Put(numberTotal, total)
	}
	score := func(input *types.Frame) {
		input.Put(numberReady, 1)
	}

	Convey("CrossSection folds peers without mutating keyed state", t, func() {
		number := NewNumber[string](numberAccumulator)
		for key, value := range map[string]float64{"focal": 2, "first": 3, "second": 5} {
			_ = number.Step(key, types.Frame{}.Set(numberDelta, value))
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
			_ = number.Step(key, types.Frame{}.Set(numberDelta, value))
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
			_ = number.Step(key, types.Frame{}.Set(numberDelta, value))
		}
		_, _, _, ready, err := number.ArgMax(score, numberTotal, numberReady)
		So(err, ShouldBeNil)
		So(ready, ShouldBeFalse)
	})
}

func TestSingleLifecycle(t *testing.T) {
	Convey("NewSingle carries state across calls without allocating a scratch frame", t, func() {
		single := NewSingle(numberAccumulator)
		input := types.Frame{}.Set(numberDelta, 1)

		first := single(input)
		So(first.Err, ShouldBeNil)
		So(first.MustGet(numberTotal), ShouldEqual, 1.0)

		second := single(input)
		So(second.Err, ShouldBeNil)
		So(second.MustGet(numberTotal), ShouldEqual, 2.0)

		// The step's own churn is what is under test, so the returned frame is
		// deliberately not bound to a local here. A Frame is MaxSlots wide, and
		// binding one inside the measured closure makes the compiler heap
		// allocate the caller's copy — charging sizeof(Frame) to the engine and
		// turning this into an assertion about the struct's size rather than
		// about NewSingle reusing its scratch frame. The error is checked on the
		// established calls above instead.
		allocations := testing.AllocsPerRun(1000, func() {
			single(input)
		})
		So(allocations, ShouldEqual, 0.0)
	})
}

func BenchmarkNumberEstablishedKey(b *testing.B) {
	number := NewNumber[string](numberAccumulator)
	input := types.Frame{}.Set(numberDelta, 1)
	_ = number.Step("symbol", input)
	b.ReportAllocs()

	for b.Loop() {
		_ = number.Step("symbol", input)
	}
}

func BenchmarkSingleStep(b *testing.B) {
	single := NewSingle(numberAccumulator)
	input := types.Frame{}.Set(numberDelta, 1)
	_ = single(input)
	b.ReportAllocs()

	for b.Loop() {
		_ = single(input)
	}
}
