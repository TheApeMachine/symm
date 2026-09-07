package store_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestKVNext(t *testing.T) {
	Convey("Given fresh and explicitly retained map sources", t, func() {
		empty := map[string]core.Primitive{}
		fresh := store.NewKV[string](transport.NewIO(core.From(empty)))
		memory := store.NewRetained(core.From(empty))
		persistent := transport.NewPipe(store.NewKV[string](memory), memory)

		for _, operation := range []core.Primitive{fresh, persistent} {
			first := tests.Drain(t, operation, transport.NewIO(core.From(map[string]core.Primitive{"a": core.From(1.0)})))[0].(map[string]core.Primitive)
			second := tests.Drain(t, operation, transport.NewIO(core.From(map[string]core.Primitive{"b": core.From(2.0)})))[0].(map[string]core.Primitive)
			_, found := second["a"]
			So(found, ShouldEqual, operation == persistent)
			So(len(first), ShouldEqual, 1)
			So(core.To[float64](first["a"]), ShouldEqual, 1)
		}
		So(len(empty), ShouldEqual, 0)
	})

	Convey("Given multiple fields and repeated keys in one finite delivery", t, func() {
		seed := map[string]core.Primitive{"existing": core.From(7.0)}
		incoming := []map[string]core.Primitive{
			{"mean": core.From(10.0)},
			{"count": core.From(3.0)},
			{"mean": core.From(-5.0)},
			{"existing": core.From(12.0)},
		}
		values := make([]core.Primitive, len(incoming))

		for index, fields := range incoming {
			values[index] = core.From(fields)
		}
		operation := store.NewKV[string](transport.NewIO(core.From(seed)))
		first := tests.Drain(t, operation, transport.NewIO(values...))[0].(map[string]core.Primitive)
		second := tests.Drain(t, operation, transport.NewIO(core.From(map[string]core.Primitive{"mean": core.From(20.0)})))[0].(map[string]core.Primitive)
		So(core.To[float64](first["mean"]), ShouldEqual, -5)
		So(core.To[float64](first["count"]), ShouldEqual, 3)
		So(core.To[float64](first["existing"]), ShouldEqual, 12)
		So(core.To[float64](second["mean"]), ShouldEqual, 20)
		So(core.To[float64](second["existing"]), ShouldEqual, 7)
		So(len(second), ShouldEqual, 2)
		So(len(seed), ShouldEqual, 1)
		So(core.To[float64](seed["existing"]), ShouldEqual, 7)
		So(core.To[float64](incoming[0]["mean"]), ShouldEqual, 10)
		So(core.To[float64](incoming[3]["existing"]), ShouldEqual, 12)
		tests.Sound(t, operation)
	})
}

func BenchmarkKVNext(b *testing.B) {
	// These are the numeric fields of a lead-lag candidate diagnostic record.
	names := [...]string{"index", "lag_index", "x", "y", "defined", "correlation", "covariance", "left_energy", "right_energy", "left_returns", "right_returns", "support", "left_rate", "right_rate", "left_spacing", "right_spacing"}
	fields := make([]core.Primitive, len(names))

	for index, name := range names {
		fields[index] = core.From(map[string]core.Primitive{name: core.From(float64(index))})
	}
	operation := store.NewKV[string](transport.NewIO(core.From(map[string]core.Primitive{})))
	input := transport.NewIO(fields...)
	b.ReportAllocs()

	for b.Loop() {
		result := operation.Next(input)

		if result == nil || operation.Next(input) != nil || operation.Error() != nil {
			b.Fatal("record merge failed", operation.Error())
		}
	}
}
