package store_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/data"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestQueryNext(t *testing.T) {
	Convey("A query issues one command after upstream work completes", t, func() {
		key := []byte("count")
		radix := store.NewRadix[float64]()
		query := store.NewKeyQuery[float64](&key, data.ActionRead)
		pipeline := nomagique.NewNumber(query)
		So(len(tests.CollectSeq[store.Query[float64]](pipeline.Next(nil))), ShouldEqual, 1)

		Convey("A write binds the actual upstream computation before a later read", func() {
			pipeline = nomagique.NewNumber(sequence.
				NewValues(7.0), store.NewKeyQuery[float64](&key, data.ActionWrite), radix,
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
