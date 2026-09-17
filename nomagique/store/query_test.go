package store_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestQueryNext(t *testing.T) {
	Convey("A query issues one command after upstream work completes", t, func() {
		key := []byte("count")
		radix := store.NewRadix[float64]()
		query := store.NewKeyQuery[float64](&key, data.ActionRead)
		pipeline := nomagique.NewNumber(query)
		So(len(tests.CollectSeq[store.Query[*[]byte, float64]](pipeline.Next(nil))), ShouldEqual, 1)

		Convey("A write binds the actual upstream computation before a later read", func() {
			pipeline = nomagique.NewNumber(sequence.NewValues(7.0), store.NewKeyQuery[float64](&key, data.ActionWrite), radix,
				query, radix,
			)
			So(tests.CollectSeq[float64](pipeline.Next(nil)), ShouldResemble, []float64{7})
			So(pipeline.Error(), ShouldBeNil)
		})

		Convey("Multiple upstream yields finish before the single command", func() {
			visited := 0
			upstream := func(yield func(unsafe.Pointer) bool) {
				for value := 0; value < 3; value++ {
					visited++

					if !yield(unsafe.Pointer(&value)) {
						return
					}
				}
			}
			commands := 0

			for range query.Next(upstream) {
				So(visited, ShouldEqual, 3)
				commands++
			}
			So(commands, ShouldEqual, 1)
		})
	})
}

func TestNewQuery(t *testing.T) {
	Convey("Identity and payload types are independent", t, func() {
		subject := store.NewQuery[string, bool](nil, data.ActionNone)
		subject.Address = "owner"
		query := store.NewQuery[string, float64](subject, data.ActionWrite, sequence.NewValue(12.5))
		So(query.Identity(), ShouldEqual, "owner")
		So(query.Address, ShouldEqual, "owner")
		So(query.First(), ShouldEqual, 12.5)
		So(query.Error(), ShouldBeNil)
	})
}

func TestQueryIdentify(t *testing.T) {
	Convey("Identification updates the typed subject and query address", t, func() {
		subject := store.NewQuery[string, bool](nil, data.ActionNone)
		query := store.NewQuery[string, float64](subject, data.ActionRead)
		So(query.Identify("new-address"), ShouldEqual, query)
		So(query.Address, ShouldEqual, "new-address")
		So(subject.Identity(), ShouldEqual, "new-address")
		subject.Identify("moved")
		So(query.Identity(), ShouldEqual, "moved")
	})
}

func TestQueryIdentity(t *testing.T) {
	Convey("Without a subject the address is the identity, including zero", t, func() {
		query := store.NewQuery[int, string](nil, data.ActionRead)
		So(query.Identity(), ShouldEqual, 0)
		query.Identify(7)
		So(query.Identity(), ShouldEqual, 7)
	})
}

func TestQueryFirst(t *testing.T) {
	Convey("First returns an identifiable member directly", t, func() {
		member := &tests.Member[*geometry.Coordinate]{Address: geometry.NewCoordinate(0, 0), Primitive: store.NewRetained(2.0)}
		query := store.NewQuery[*geometry.Coordinate, core.Identifiable[*geometry.Coordinate]](
			nil, data.ActionIdentify, sequence.NewValue[core.Identifiable[*geometry.Coordinate]](member),
		)
		var result core.Identifiable[*geometry.Coordinate] = query.First()
		So(result, ShouldEqual, member)
	})
}

func BenchmarkQueryNext(b *testing.B) {
	b.Run("ConfiguredPayload", func(b *testing.B) {
		query := store.NewQuery[string, float64](nil, data.ActionWrite, sequence.NewValue(12.5))
		query.Address = "value"
		run := query.Next(nil)
		b.ReportAllocs()
		for b.Loop() {
			for output := range run {
				if (*store.Query[string, float64])(output).First() != 12.5 {
					b.Fatal("payload changed")
				}
			}
		}
	})
	b.Run("UpstreamPayload", func(b *testing.B) {
		query := store.NewQuery[string, float64](nil, data.ActionWrite)
		query.Address = "value"
		run := query.Next(sequence.NewValue(12.5))
		b.ReportAllocs()
		for b.Loop() {
			for output := range run {
				if (*store.Query[string, float64])(output).First() != 12.5 {
					b.Fatal("payload changed")
				}
			}
		}
	})
}

func TestQueryNextBorrow(t *testing.T) {
	Convey("An input-bound query lends its own address and releases the payload after consumption", t, func() {
		query := store.NewQuery[string, float64](nil, data.ActionExecute)
		query.Address = "metric"
		for _, value := range []float64{3, 0, -2} {
			for output := range query.Next(sequence.NewOne(unsafe.Pointer(&value)).Next(nil)) {
				So(output, ShouldEqual, unsafe.Pointer(query))
				So((*store.Query[string, float64])(output).First(), ShouldEqual, value)
				break
			}
			So(query.First(), ShouldEqual, 0)
		}
	})
}
