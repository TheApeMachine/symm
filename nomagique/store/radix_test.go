package store

import (
	"testing"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestRadixNext(t *testing.T) {
	Convey("Radix retains writes and answers a prefix selector from its own state", t, func() {
		op := NewRadix(iradix.New[[]byte]())

		tests.CollectSeq(op.Next(transport.Values(
			map[string][]byte{"selector": []byte("b/enter\x00abc"), "data": []byte("one")},
			map[string][]byte{"selector": []byte("b/exit\x00abc"), "data": []byte("two")},
			map[string][]byte{"selector": []byte("b/")},
		)))

		tree := op.Read()
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
