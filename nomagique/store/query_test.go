package store_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestQueryNext(t *testing.T) {
	Convey("A query yields itself and forwards arriving input", t, func() {
		key := []byte("count")
		query := store.NewKeyQuery[float64](&key, core.Read)
		value := 42.0
		count := 0
		var receivedQuery *store.Query[*[]byte, float64]
		var receivedVal float64

		for ptr := range query.Next(sequence.NewOne(unsafe.Pointer(&value)).Next(nil)) {
			if count == 0 {
				receivedQuery = (*store.Query[*[]byte, float64])(ptr)
			}

			if count == 1 {
				receivedVal = *(*float64)(ptr)
			}

			count++
		}

		So(count, ShouldEqual, 2)
		So(receivedQuery, ShouldEqual, query)
		So(receivedVal, ShouldEqual, 42.0)
	})
}

func TestNewQuery(t *testing.T) {
	Convey("NewQuery binds an address and action", t, func() {
		addr := transport.NewAddress[string]()
		addr.Identify("owner")
		query := store.NewQuery[string, float64](addr, core.Write)

		So(query.Identity(), ShouldEqual, "owner")
		So(query.Action, ShouldEqual, core.Write)
		So(query.Error(), ShouldBeNil)
	})
}

func TestQueryIdentify(t *testing.T) {
	Convey("Identification updates the address identity", t, func() {
		addr := transport.NewAddress[string]()
		query := store.NewQuery[string, float64](addr, core.Read)
		query.Identify("new-address")
		So(query.Identity(), ShouldEqual, "new-address")
	})
}
