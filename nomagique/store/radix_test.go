package store_test

import (
	"testing"
	"unsafe"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestRadixNext(t *testing.T) {
	Convey("Radix retains writes and answers a prefix selector from its own state", t, func() {
		op := store.NewRadix(iradix.New[[]byte]())

		m1 := map[string][]byte{"selector": []byte("b/enter\x00abc"), "data": []byte("one")}
		m2 := map[string][]byte{"selector": []byte("b/exit\x00abc"), "data": []byte("two")}
		m3 := map[string][]byte{"selector": []byte("b/")}

		in := func(yield func(unsafe.Pointer) bool) {
			for _, m := range []map[string][]byte{m1, m2, m3} {
				if !yield(unsafe.Pointer(&m)) {
					return
				}
			}
		}

		out := tests.CollectSeq[*iradix.Tree[[]byte]](op.Next(in))
		So(len(out), ShouldEqual, 3)

		tree := out[len(out)-1]
		So(tree, ShouldNotBeNil)

		found := map[string]string{}
		iterator := tree.Root().Iterator()
		iterator.SeekPrefix([]byte("b/"))

		for key, value, more := iterator.Next(); more; key, value, more = iterator.Next() {
			found[string(key)] = string(value)
		}

		So(found["b/enter\x00abc"], ShouldEqual, "one")
		So(found["b/exit\x00abc"], ShouldEqual, "two")
		So(len(found), ShouldEqual, 2)
	})
}
