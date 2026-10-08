package transport_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/probability"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestValuesNext(t *testing.T) {
	Convey("Values ignores the inbound run and yields each held value", t, func() {
		op := transport.NewValues(1.0, 2.0, 3.0)
		out := tests.CollectSeq[float64](op.Next(nil))

		So(out, ShouldResemble, []float64{1, 2, 3})
		So(op.Error(), ShouldBeNil)
	})

	Convey("Repeated Next calls re-yield a fresh run", t, func() {
		op := transport.NewValues(4.0, 7.0)
		in := transport.NewValues(9.0).Next(nil)

		So(tests.CollectSeq[float64](op.Next(in)), ShouldResemble, []float64{4, 7})
		So(tests.CollectSeq[float64](op.Next(nil)), ShouldResemble, []float64{4, 7})
	})
}

func TestOneNext(t *testing.T) {
	Convey("One ignores the inbound run and yields the held pointer", t, func() {
		value := 5.0
		op := transport.NewOne(unsafe.Pointer(&value))
		out := tests.CollectSeq[float64](op.Next(transport.NewValues(1.0).Next(nil)))

		So(out, ShouldResemble, []float64{5})
		So(op.Error(), ShouldBeNil)
	})
}

/*
TestValuesOwnsStorage proves the source copies a variadic spread: a variadic
spread of an existing slice aliases the caller's backing array, and mutating
primitives must never write through into the caller's data.
*/
func TestValuesOwnsStorage(t *testing.T) {
	evidence := []float64{1, 1, 1.8}

	for out := range probability.NewShannonAmbiguity().Next(transport.NewValues(evidence...).Next(nil)) {
		_ = *(*float64)(out)
	}

	if evidence[0] != 1 || evidence[2] != 1.8 {
		t.Fatalf("source aliased caller storage: %v", evidence)
	}
}
