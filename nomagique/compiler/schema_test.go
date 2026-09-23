package compiler

import (
	"math"
	"testing"

	capnp "capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/std/capnp/schema"
	. "github.com/smartystreets/goconvey/convey"
)

func TestCompileFanOutCopier(t *testing.T) {
	Convey("Given typed list slots projected into real Cap'n Proto fields", t, func() {
		_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
		So(err, ShouldBeNil)
		source, err := capnp.NewRootStruct(segment, capnp.ObjectSize{PointerCount: 1})
		So(err, ShouldBeNil)
		destination, err := capnp.NewStruct(segment, capnp.ObjectSize{DataSize: 8, PointerCount: 1})
		So(err, ShouldBeNil)

		Convey("Then integer identities are copied without conversion through Float64", func() {
			values, err := capnp.NewUInt64List(segment, 2)
			So(err, ShouldBeNil)
			values.Set(1, 9007199254740993)
			So(source.SetPtr(0, values.ToPtr()), ShouldBeNil)
			copy, err := CompileFanOutCopier(FieldInfo{ElementWhich: schema.Type_Which_uint64}, FieldInfo{Which: schema.Type_Which_uint64}, 1, FieldInfo{}, false)
			So(err, ShouldBeNil)
			So(copy(source, destination), ShouldBeNil)
			So(destination.Uint64(0) == uint64(9007199254740993), ShouldBeTrue)
		})

		Convey("Then a false Boolean remains a delivered false rather than an absent slot", func() {
			values, err := capnp.NewBitList(segment, 2)
			So(err, ShouldBeNil)
			values.Set(0, true)
			So(source.SetPtr(0, values.ToPtr()), ShouldBeNil)
			field := FieldInfo{ElementWhich: schema.Type_Which_bool}
			copy, err := CompileFanOutCopier(field, FieldInfo{Which: schema.Type_Which_bool, Offset: 3}, 1, FieldInfo{}, false)
			So(err, ShouldBeNil)
			destination.SetBit(3, true)
			So(copy(source, destination), ShouldBeNil)
			So(destination.Bit(3), ShouldBeFalse)
			So(CompileFanOutDelivery(field, 1, FieldInfo{}, false)(source), ShouldBeTrue)
			So(CompileFanOutDelivery(field, 2, FieldInfo{}, false)(source), ShouldBeFalse)
		})

		Convey("Then a Data slot preserves its bytes", func() {
			values, err := capnp.NewDataList(segment, 2)
			So(err, ShouldBeNil)
			So(values.Set(1, []byte(`{"count":9007199254740993}`)), ShouldBeNil)
			So(source.SetPtr(0, values.ToPtr()), ShouldBeNil)
			copy, err := CompileFanOutCopier(FieldInfo{ElementWhich: schema.Type_Which_data}, FieldInfo{Which: schema.Type_Which_data}, 1, FieldInfo{}, false)
			So(err, ShouldBeNil)
			So(copy(source, destination), ShouldBeNil)
			result, err := destination.Ptr(0)
			So(err, ShouldBeNil)
			So(string(result.Data()), ShouldEqual, `{"count":9007199254740993}`)
		})
	})
}

func TestSetStaticField(t *testing.T) {
	Convey("Given an explicit list of Text in graph configuration", t, func() {
		_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
		So(err, ShouldBeNil)
		target, err := capnp.NewRootStruct(segment, capnp.ObjectSize{PointerCount: 1})
		So(err, ShouldBeNil)
		field := FieldInfo{Which: schema.Type_Which_list, ElementWhich: schema.Type_Which_text}
		So(SetStaticField(target, field, `["correlation:ticker"]`), ShouldBeNil)
		pointer, err := target.Ptr(0)
		So(err, ShouldBeNil)
		text, err := capnp.TextList(pointer.List()).At(0)
		So(err, ShouldBeNil)
		So(text, ShouldEqual, "correlation:ticker")
		So(SetStaticField(target, field, `null`), ShouldNotBeNil)
		So(SetStaticField(target, field, `[1]`), ShouldNotBeNil)
		So(SetStaticField(target, field, `[null]`), ShouldNotBeNil)
	})
}

func TestCompileFanInSlotCopier(t *testing.T) {
	Convey("Given indexed edges gathering different slots of a batched result", t, func() {
		_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
		So(err, ShouldBeNil)
		source, err := capnp.NewRootStruct(segment, capnp.ObjectSize{PointerCount: 1})
		So(err, ShouldBeNil)
		destination, err := capnp.NewStruct(segment, capnp.ObjectSize{PointerCount: 1})
		So(err, ShouldBeNil)
		values, err := capnp.NewDataList(segment, 2)
		So(err, ShouldBeNil)
		So(values.Set(0, []byte("counts")), ShouldBeNil)
		So(values.Set(1, []byte("example")), ShouldBeNil)
		So(source.SetPtr(0, values.ToPtr()), ShouldBeNil)
		from := FieldInfo{Which: schema.Type_Which_list, ElementWhich: schema.Type_Which_data, ValueList: true}
		to := from
		for index := range 2 {
			copier, err := CompileFanInSlotCopier(from, to, 1-index, index, 2)
			So(err, ShouldBeNil)
			So(copier(source, destination), ShouldBeNil)
		}
		pointer, err := destination.Ptr(0)
		So(err, ShouldBeNil)
		actual := capnp.DataList(pointer.List())
		So(actual.Len(), ShouldEqual, 2)
		first, err := actual.At(0)
		So(err, ShouldBeNil)
		second, err := actual.At(1)
		So(err, ShouldBeNil)
		So(string(first), ShouldEqual, "example")
		So(string(second), ShouldEqual, "counts")
		Convey("Incompatible slots fail at compile time", func() {
			to.ElementWhich = schema.Type_Which_text
			_, err := CompileFanInSlotCopier(from, to, 0, 0, 1)
			So(err, ShouldNotBeNil)
		})
	})
}

func TestCompileFanInCopier(t *testing.T) {
	Convey("Given numeric signal results gathered by a typed list input", t, func() {
		_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
		So(err, ShouldBeNil)
		source, err := capnp.NewRootStruct(segment, capnp.ObjectSize{DataSize: 16})
		So(err, ShouldBeNil)
		target, err := capnp.NewStruct(segment, capnp.ObjectSize{PointerCount: 1})
		So(err, ShouldBeNil)
		from := FieldInfo{Which: schema.Type_Which_float64, Offset: 1}
		to := FieldInfo{Which: schema.Type_Which_list, ElementWhich: schema.Type_Which_float64}
		for index, value := range []float64{2.5, -7, 0, 19} {
			source.SetUint64(8, math.Float64bits(value))
			copy, err := CompileFanInCopier(from, to, index, 4)
			So(err, ShouldBeNil)
			So(copy(source, target), ShouldBeNil)
		}
		pointer, err := target.Ptr(0)
		So(err, ShouldBeNil)
		values := capnp.Float64List(pointer.List())
		So(values.Len(), ShouldEqual, 4)
		for index, value := range []float64{2.5, -7, 0, 19} {
			So(values.At(index), ShouldEqual, value)
		}
	})
}

func BenchmarkCompileFanInCopier(b *testing.B) {
	copy, err := CompileFanInCopier(FieldInfo{Which: schema.Type_Which_float64}, FieldInfo{Which: schema.Type_Which_list, ElementWhich: schema.Type_Which_float64}, 2, 411)
	if err != nil {
		b.Fatal(err)
	}
	_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		b.Fatal(err)
	}
	source, err := capnp.NewRootStruct(segment, capnp.ObjectSize{DataSize: 8})
	if err != nil {
		b.Fatal(err)
	}
	source.SetUint64(0, math.Float64bits(12.5))
	b.ReportAllocs()
	for b.Loop() {
		// Each graph evaluation owns a fresh argument message and gathering list.
		message, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
		if err != nil {
			b.Fatal(err)
		}
		target, err := capnp.NewRootStruct(segment, capnp.ObjectSize{PointerCount: 1})
		if err != nil {
			b.Fatal(err)
		}
		if err := copy(source, target); err != nil {
			b.Fatal(err)
		}
		message.Release()
	}
}
