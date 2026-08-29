package adaptive

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/types"
)

const (
	testValue = "test/value"
	testReady = "test/ready"
	testCount = "test/count"
	testScore = "test/z/value"
)

func withValue(name string, value float64) types.Frame {
	frame := types.Frame{}
	frame.Put(types.MustIntern(name), value)

	return frame
}

func TestStandardizer(t *testing.T) {
	Convey("Given a standardizer primitive over a prefixed series", t, func() {
		standardizer := Standardizer("test")

		Convey("It reports not ready before dispersion exists", func() {
			number := nomagique.NewNumber[string](standardizer)

			output := number.Step("sym", withValue(testValue, 10))

			So(output.Err, ShouldBeNil)
			So(output.MustGet(types.MustIntern(testReady)), ShouldEqual, 0.0)
			So(output.MustGet(types.MustIntern(testCount)), ShouldEqual, 1.0)
			So(output.MustGet(types.MustIntern(testScore)), ShouldEqual, 0.0)
		})

		Convey("It scores against prior moments once dispersion is positive", func() {
			number := nomagique.NewNumber[string](standardizer)

			_ = number.Step("sym", withValue(testValue, 1))
			_ = number.Step("sym", withValue(testValue, 2))
			output := number.Step("sym", withValue(testValue, 3))

			So(output.Err, ShouldBeNil)
			So(output.MustGet(types.MustIntern(testReady)), ShouldEqual, 1.0)
			So(output.MustGet(types.MustIntern(testScore)), ShouldBeGreaterThan, 0.0)
		})

		Convey("It rejects a missing value", func() {
			output := types.Frame{}
			standardizer(&output)
			So(output.Err, ShouldNotBeNil)
		})
	})
}

func BenchmarkStandardizer(benchmark *testing.B) {
	standardizer := Standardizer("bench")

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		output := withValue("bench/value", 1.5)
		standardizer(&output)
	}
}
