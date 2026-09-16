package core_test

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
)

func TestPrimitiveErrorError(t *testing.T) {
	Convey("A composed error owner preserves construction and execution failures", t, func() {
		construction := errors.New("construction failed")
		execution := errors.New("execution failed")
		primitiveError := core.NewPrimitiveError(construction)

		Convey("Construction errors are visible immediately", func() {
			So(errors.Is(primitiveError.Error(), construction), ShouldBeTrue)
		})

		Convey("Later errors retain both causes and nil adds nothing", func() {
			primitiveError.Error(nil, execution)
			So(errors.Is(primitiveError.Error(), construction), ShouldBeTrue)
			So(errors.Is(primitiveError.Error(), execution), ShouldBeTrue)
			recorded := primitiveError.Error()
			So(primitiveError.Error(nil), ShouldEqual, recorded)
		})

		Convey("An empty owner starts without an error", func() {
			So(core.NewPrimitiveError().Error(), ShouldBeNil)
		})
	})
}

func BenchmarkPrimitiveErrorError(b *testing.B) {
	primitiveError := core.NewPrimitiveError()
	b.ReportAllocs()
	for b.Loop() {
		if err := primitiveError.Error(); err != nil {
			b.Fatal(err)
		}
	}
}
