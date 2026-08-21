package nomagique

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

var numberTestTotal = MustIntern("test/number/total")
var numberTestReady = MustIntern("test/number/ready")

func TestNumber(t *testing.T) {
	Convey("Given a keyed Number", t, func() {
		number := NewNumber[string](numberTestSum)

		_, err := number.Step("left", Frame{}.Set(SampleValue, 2))
		So(err, ShouldBeNil)
		_, err = number.Step("left", Frame{}.Set(SampleValue, 3))
		So(err, ShouldBeNil)
		_, err = number.Step("right", Frame{}.Set(SampleValue, 7))
		So(err, ShouldBeNil)

		Convey("It should retain isolated state for every key", func() {
			left, found := number.Project("left")
			So(found, ShouldBeTrue)
			So(left.MustGet(numberTestTotal), ShouldEqual, 5.0)

			right, found := number.Project("right")
			So(found, ShouldBeTrue)
			So(right.MustGet(numberTestTotal), ShouldEqual, 7.0)
		})

		Convey("It should range every committed projection", func() {
			total := 0.0
			count := 0
			number.Range(func(_ string, state Frame) bool {
				total += state.MustGet(numberTestTotal)
				count++

				return true
			})

			So(count, ShouldEqual, 2)
			So(total, ShouldEqual, 12.0)
		})
	})
}

func TestNumberCrossSection(t *testing.T) {
	Convey("Given three committed keyed states", t, func() {
		number := NewNumber[string](numberTestSum)

		for key, value := range map[string]float64{"focal": 2, "first": 3, "second": 5} {
			_, err := number.Step(key, Frame{}.Set(SampleValue, value))
			So(err, ShouldBeNil)
		}

		output, ready, err := number.CrossSection(
			"focal",
			numberTestPair,
			numberTestReduce,
			Identity,
		)

		Convey("It should fold every peer without mutating keyed state", func() {
			So(err, ShouldBeNil)
			So(ready, ShouldBeTrue)
			So(output.MustGet(numberTestTotal), ShouldEqual, 12.0)

			focal, found := number.Project("focal")
			So(found, ShouldBeTrue)
			So(focal.MustGet(numberTestTotal), ShouldEqual, 2.0)
		})
	})
}

func TestNumberArgMax(t *testing.T) {
	Convey("Given a unique cross-sectional leader above the median", t, func() {
		number := NewNumber[string](numberTestSum)

		for key, value := range map[string]float64{"low": 1, "middle": 2, "leader": 5} {
			_, err := number.Step(key, Frame{}.Set(SampleValue, value))
			So(err, ShouldBeNil)
		}

		selected, maximum, median, ready, err := number.ArgMax(
			numberTestScore,
			numberTestTotal,
			numberTestReady,
		)

		So(err, ShouldBeNil)
		So(ready, ShouldBeTrue)
		So(selected, ShouldEqual, "leader")
		So(maximum, ShouldEqual, 5.0)
		So(median, ShouldEqual, 2.0)
	})
}

func numberTestSum(
	state Frame,
	input Frame,
) (Frame, Frame, error) {
	value := input.MustGet(SampleValue)
	total, _ := state.Get(numberTestTotal)
	nextState := state
	nextState.Put(numberTestTotal, total+value)

	return nextState, nextState, nil
}

func numberTestPair(
	state Frame,
	input Frame,
) (Frame, Frame, error) {
	output := Frame{}
	output.Put(
		numberTestTotal,
		state.MustGet(numberTestTotal)+input.MustGet(numberTestTotal),
	)

	return state, output, nil
}

func numberTestReduce(
	state Frame,
	input Frame,
) (Frame, Frame, error) {
	total, _ := state.Get(numberTestTotal)
	total += input.MustGet(numberTestTotal)
	state.Put(numberTestTotal, total)

	return state, state, nil
}

func numberTestScore(
	state Frame,
	input Frame,
) (Frame, Frame, error) {
	input.Put(numberTestReady, 1)

	return state, input, nil
}

func BenchmarkNumber(benchmark *testing.B) {
	number := NewNumber[string](numberTestSum)
	input := Frame{}.Set(SampleValue, 1)
	_, _ = number.Step("symbol", input)
	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_, _ = number.Step("symbol", input)
	}
}

func BenchmarkNumberCrossSection(benchmark *testing.B) {
	number := NewNumber[string](numberTestSum)
	input := Frame{}.Set(SampleValue, 1)

	for _, key := range []string{"focal", "first", "second"} {
		_, _ = number.Step(key, input)
	}

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_, _, _ = number.CrossSection(
			"focal",
			numberTestPair,
			numberTestReduce,
			Identity,
		)
	}
}

func BenchmarkNumberArgMax(benchmark *testing.B) {
	number := NewNumber[string](numberTestSum)

	for _, key := range []string{"low", "middle", "leader"} {
		_, _ = number.Step(key, Frame{}.Set(SampleValue, float64(len(key))))
	}

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_, _, _, _, _ = number.ArgMax(
			numberTestScore,
			numberTestTotal,
			numberTestReady,
		)
	}
}
