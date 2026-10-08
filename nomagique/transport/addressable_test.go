package transport_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestAddressable(t *testing.T) {
	Convey("Addressable passes data through space", t, func() {
		addr := transport.NewAddressable("test", store.NewKV())

		val := 42.0
		msg := data.NewMessage(data.WRITE, "test", "key", data.NewValue(val))

		var results []float64
		for ptr := range addr.Next(msg.Next(nil)) {
			results = append(results, *(*float64)(ptr))
		}

		So(len(results), ShouldEqual, 1)
		So(results[0], ShouldEqual, 42.0)
	})
}
