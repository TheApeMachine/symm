package core_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
)

func TestFrom(t *testing.T) {
	Convey("Boundary lifting preserves payload and primitive identity", t, func() {
		primitive := core.NewProto(7.0)
		Convey("Concrete and interface primitives pass through", func() {
			So(core.From(primitive), ShouldEqual, primitive)
			So(core.From[core.Primitive](primitive), ShouldEqual, primitive)
			So(core.From[any](primitive), ShouldEqual, primitive)
		})
		Convey("Nil primitive and untyped nil retain distinct meanings", func() {
			So(core.From[core.Primitive](nil), ShouldBeNil)
			untyped := core.From[any](nil)
			So(untyped, ShouldNotBeNil)
			So(untyped.Read(), ShouldBeNil)
			var pointer *core.Proto
			lifted := core.From(pointer)
			So(lifted == nil, ShouldBeFalse)
			So(lifted, ShouldEqual, pointer)
		})
		Convey("Successive boundary values remain independently readable", func() {
			first := core.From(3.0)
			second := core.From(9.0)
			So(core.To[float64](first), ShouldEqual, 3)
			So(core.To[float64](second), ShouldEqual, 9)
		})
	})
}

var liftedBoundary core.Primitive

func BenchmarkFrom(b *testing.B) {
	b.Run("numeric", func(b *testing.B) {
		for b.Loop() {
			liftedBoundary = core.From(7.0)
		}
	})
	b.Run("primitive", func(b *testing.B) {
		primitive := core.NewProto(7.0)
		for b.Loop() {
			liftedBoundary = core.From[core.Primitive](primitive)
		}
	})
	b.Run("record", func(b *testing.B) {
		for b.Loop() {
			liftedBoundary = core.Record(map[string]any{
				"price": 7.0, "quantity": 3.0, "epoch": uint64(9), "symbol": "BTC/USD",
			})
		}
	})
}
