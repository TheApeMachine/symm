package temporal

import (
	"math"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPathReturns(t *testing.T) {
	Convey("PathReturns decodes sequential prices into log returns and cumulative energy", t, func() {
		pr := NewPathReturns()
		path := &PricePath{
			Prices: []Price{
				{At: 100, Value: 10.0},
				{At: 200, Value: 20.0},
				{At: 300, Value: 15.0},
			},
		}

		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(path))
		}

		var collected []ReturnPath

		for ptr := range pr.Next(in) {
			collected = append(collected, *(*ReturnPath)(ptr))
		}

		So(pr.Error(), ShouldBeNil)
		So(collected, ShouldHaveLength, 1)

		retPath := collected[0]
		So(retPath.Returns, ShouldHaveLength, 2)

		expectedFirst := math.Log(20.0) - math.Log(10.0)
		expectedSecond := math.Log(15.0) - math.Log(20.0)

		So(retPath.Returns[0].From, ShouldEqual, 100)
		So(retPath.Returns[0].To, ShouldEqual, 200)
		So(retPath.Returns[0].Value, ShouldAlmostEqual, expectedFirst, 1e-9)

		So(retPath.Returns[1].From, ShouldEqual, 200)
		So(retPath.Returns[1].To, ShouldEqual, 300)
		So(retPath.Returns[1].Value, ShouldAlmostEqual, expectedSecond, 1e-9)

		expectedEnergy := expectedFirst*expectedFirst + expectedSecond*expectedSecond
		So(retPath.Energy, ShouldAlmostEqual, expectedEnergy, 1e-9)
	})
}
