package compiler

import (
	"encoding/json"
	"math"
	"testing"

	capnp "capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/schemas"
	"capnproto.org/go/capnp/v3/std/capnp/schema"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/compiler/testdata/projectionfixture"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/types"
)

func init() { projectionfixture.RegisterSchema(schemas.DefaultRegistry) }

func TestResultProjectionStruct(t *testing.T) {
	Convey("Given generated wire values with schema defaults", t, func() {
		_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
		So(err, ShouldBeNil)
		value, err := projectionfixture.NewRootValue(segment)
		So(err, ShouldBeNil)
		projection := &resultProjection{nodes: make(map[uint64]schema.Node)}
		project := func() map[string]any {
			result, err := projection.Struct(capnp.Struct(value), projectionfixture.Value_TypeID)
			So(err, ShouldBeNil)
			return result
		}
		Convey("Defaults retain meaning and 64-bit integers retain every digit", func() {
			result := project()
			So(result["label"], ShouldEqual, "default label")
			So(result["amount"], ShouldEqual, 12.5)
			So(result["count"], ShouldEqual, "18446744073709551615")
			So(result["signed"], ShouldEqual, "-9223372036854775808")
			So(result, ShouldContainKey, "missing")
			So(result, ShouldNotContainKey, "found")
			So(result["child"], ShouldBeNil)
			value.SetAmount(-7.25)
			value.SetCount(9007199254740993)
			So(project()["amount"], ShouldEqual, -7.25)
			encoded, err := json.Marshal(project())
			So(err, ShouldBeNil)
			So(string(encoded), ShouldContainSubstring, `"count":"9007199254740993"`)
		})
		Convey("Nested lists, structs, groups, and binary values survive projection", func() {
			points, err := value.NewPoints(3)
			So(err, ShouldBeNil)
			points.Set(0, -12)
			points.Set(1, 0)
			points.Set(2, 24)
			children, err := value.NewChildren(2)
			So(err, ShouldBeNil)
			children.At(0).SetAmount(10)
			children.At(1).SetFound()
			children.At(1).Found().SetScore(8)
			groups, err := value.NewGroups(1)
			So(err, ShouldBeNil)
			labels, err := capnp.NewTextList(segment, 2)
			So(err, ShouldBeNil)
			So(labels.Set(0, "first"), ShouldBeNil)
			So(labels.Set(1, "second"), ShouldBeNil)
			So(groups.Set(0, labels.ToPtr()), ShouldBeNil)
			So(value.SetBlob([]byte{0, 255, 10}), ShouldBeNil)
			result := project()
			So(result["points"], ShouldResemble, []any{float64(-12), float64(0), float64(24)})
			So(result["groups"], ShouldResemble, []any{[]any{"first", "second"}})
			nested := result["children"].([]any)
			So(nested[0].(map[string]any)["amount"], ShouldEqual, 10)
			found := nested[1].(map[string]any)
			So(found, ShouldNotContainKey, "missing")
			So(found["found"], ShouldResemble, map[string]any{"score": float32(8), "visible": true})
			// Returned bytes belong to the response, not the soon-released RPC message.
			blob, err := value.Blob()
			So(err, ShouldBeNil)
			blob[0] = 99
			So(result["blob"], ShouldResemble, []byte{0, 255, 10})
			encoded, err := json.Marshal(result)
			So(err, ShouldBeNil)
			So(string(encoded), ShouldContainSubstring, `"blob":"AP8K"`)
			value.SetFound()
			So(project(), ShouldNotContainKey, "missing")
			value.SetMissing()
			So(project(), ShouldNotContainKey, "found")
		})
	})
}

func TestResultProjectionPointer(t *testing.T) {
	Convey("Given a capability rather than serializable data", t, func() {
		_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
		So(err, ShouldBeNil)
		valueType, err := schema.NewType(segment)
		So(err, ShouldBeNil)
		valueType.SetInterface()
		projection := &resultProjection{nodes: make(map[uint64]schema.Node)}
		_, err = projection.Pointer(capnp.Ptr{}, valueType)
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "cannot be projected as UI data")
	})
}

func TestResultProjectionField(t *testing.T) {
	Convey("Given a malformed pointer in a typed output slot", t, func() {
		_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
		So(err, ShouldBeNil)
		value, err := projectionfixture.NewRootValue(segment)
		So(err, ShouldBeNil)
		projection := &resultProjection{nodes: make(map[uint64]schema.Node)}
		node, err := projection.Node(projectionfixture.Value_TypeID)
		So(err, ShouldBeNil)
		fields, err := node.StructNode().Fields()
		So(err, ShouldBeNil)
		// label is Text, but this slot contains a struct pointer.
		So(capnp.Struct(value).SetPtr(0, value.ToPtr()), ShouldBeNil)
		_, err = projection.Field(capnp.Struct(value), fields.At(0))
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "expected text pointer")
	})
}

func BenchmarkResultProjectionStruct(b *testing.B) {
	message, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		b.Fatal(err)
	}
	value, err := projectionfixture.NewRootValue(segment)
	if err != nil {
		b.Fatal(err)
	}
	// A 64-point viewport with 16 child records exercises real nested wire data.
	points, err := value.NewPoints(64)
	if err != nil {
		b.Fatal(err)
	}
	for index := range points.Len() {
		points.Set(index, float64(index))
	}
	children, err := value.NewChildren(16)
	if err != nil {
		b.Fatal(err)
	}
	for index := range children.Len() {
		children.At(index).SetAmount(float64(index))
	}
	projection := &resultProjection{nodes: make(map[uint64]schema.Node)}
	if _, err := projection.Struct(capnp.Struct(value), projectionfixture.Value_TypeID); err != nil {
		b.Fatal(err)
	}
	// Repeated benchmark reads share an immutable wire fixture, unlike executions.
	message.ResetReadLimit(math.MaxUint64)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := projection.Struct(capnp.Struct(value), projectionfixture.Value_TypeID); err != nil {
			b.Fatal(err)
		}
	}
}

func TestResultProjectionStructRecord(t *testing.T) {
	Convey("Native metric cuts keep partial presence and exact causal stamps in the UI", t, func() {
		message, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
		So(err, ShouldBeNil)
		defer message.Release()
		cut, err := data.NewMetricCut(segment)
		So(err, ShouldBeNil)
		cut.SetEpoch(9007199254740993)
		cut.SetSequence(11)
		So(cut.SetSymbol("BTC/USD"), ShouldBeNil)
		metrics, err := cut.NewMetrics(2)
		So(err, ShouldBeNil)
		So(metrics.At(0).SetIdentity("trade:imbalance"), ShouldBeNil)
		metrics.At(0).SetPresent(true)
		metrics.At(0).SetValue(-0.25)
		metrics.At(0).SetEpoch(cut.Epoch())
		metrics.At(0).SetSequence(9)
		So(metrics.At(1).SetIdentity("ticker:correlation"), ShouldBeNil)
		record, err := types.NewRecord(segment)
		So(err, ShouldBeNil)
		record.SetTypeId(data.MetricCut_TypeID)
		So(record.SetValue(capnp.Struct(cut).ToPtr()), ShouldBeNil)
		projection := &resultProjection{nodes: make(map[uint64]schema.Node)}
		result, err := projection.Struct(capnp.Struct(record), types.Record_TypeID)
		So(err, ShouldBeNil)
		projected := result["value"].(map[string]any)
		So(projected["epoch"], ShouldEqual, "9007199254740993")
		So(projected["complete"], ShouldBeFalse)
		readings := projected["metrics"].([]any)
		So(readings[0].(map[string]any)["value"], ShouldEqual, -0.25)
		So(readings[0].(map[string]any)["sequence"], ShouldEqual, "9")
		So(readings[1].(map[string]any)["present"], ShouldBeFalse)
		cut.SetComplete(true)
		metrics.At(1).SetPresent(true)
		metrics.At(1).SetValue(0.8)
		result, err = projection.Struct(capnp.Struct(record), types.Record_TypeID)
		So(err, ShouldBeNil)
		So(result["value"].(map[string]any)["complete"], ShouldBeTrue)
		Convey("An unregistered type is an explicit failure", func() {
			record.SetTypeId(0)
			_, err := projection.Struct(capnp.Struct(record), types.Record_TypeID)
			So(err, ShouldNotBeNil)
		})
	})
}

/* BenchmarkResultProjectionStructRecord projects the shipping cut shape at the UI boundary. */
func BenchmarkResultProjectionStructRecord(b *testing.B) {
	_, input, _ := stageBindingFixture(b)
	message := input.Message()
	cut, err := data.NewMetricCut(input.Segment())
	if err != nil {
		b.Fatal(err)
	}
	bindings, err := input.Bindings()
	if err != nil {
		b.Fatal(err)
	}
	metrics, err := cut.NewMetrics(int32(bindings.Len()))
	if err != nil {
		b.Fatal(err)
	}
	for index := range metrics.Len() {
		identity, err := bindings.At(index).Node()
		if err != nil {
			b.Fatal(err)
		}
		if err := metrics.At(index).SetIdentity(identity); err != nil {
			b.Fatal(err)
		}
		metrics.At(index).SetPresent(index%2 == 0)
		metrics.At(index).SetValue(float64(index))
		metrics.At(index).SetEpoch(9007199254740993)
		metrics.At(index).SetSequence(int64(index))
	}
	record, err := types.NewRecord(input.Segment())
	if err != nil {
		b.Fatal(err)
	}
	record.SetTypeId(data.MetricCut_TypeID)
	if err := record.SetValue(capnp.Struct(cut).ToPtr()); err != nil {
		b.Fatal(err)
	}
	projection := &resultProjection{nodes: make(map[uint64]schema.Node)}
	message.ResetReadLimit(math.MaxUint64)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := projection.Struct(capnp.Struct(record), types.Record_TypeID); err != nil {
			b.Fatal(err)
		}
	}
}
