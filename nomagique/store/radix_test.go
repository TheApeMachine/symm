package store_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/store"
)

func TestRadix(t *testing.T) {
	Convey("An explicit query/store/add/query/store composition owns addressed counts", t, func() {
		radix := store.NewRadix[float64]()
		key := []byte("BTC/USD:enter")

		for _, expected := range []float64{1, 2, 3} {
			current := radix(store.RadixCommandData[float64]{
				Key:    key,
				Value:  0.0,
				Action: store.Identify,
			})
			So(current, ShouldNotBeNil)

			updatedVal := *current + 1.0
			written := radix(store.RadixCommandData[float64]{
				Key:    key,
				Value:  updatedVal,
				Action: store.Write,
			})
			So(written, ShouldNotBeNil)
			So(*written, ShouldEqual, expected)
		}

		Convey("A different address has independent evidence", func() {
			ethKey := []byte("ETH/USD:enter")
			resEth := radix(store.RadixCommandData[float64]{
				Key:    ethKey,
				Value:  1.0,
				Action: store.Write,
			})
			So(*resEth, ShouldEqual, 1.0)

			btcRead := radix(store.RadixCommandData[float64]{
				Key:    key,
				Action: store.Read,
			})
			So(btcRead, ShouldNotBeNil)
			So(*btcRead, ShouldEqual, 3.0)
		})

		Convey("An unseen read remains absent and does not create evidence", func() {
			missing := radix(store.RadixCommandData[float64]{
				Key:    []byte("missing"),
				Action: store.Read,
			})
			So(missing, ShouldBeNil)
		})

		Convey("Reusing the caller's address buffer cannot mutate published keys", func() {
			tempKey := make([]byte, len(key))
			copy(tempKey, key)
			written := radix(store.RadixCommandData[float64]{
				Key:    tempKey,
				Value:  42.0,
				Action: store.Write,
			})
			So(written, ShouldNotBeNil)
			So(*written, ShouldEqual, 42.0)

			copy(tempKey, []byte("MODIFIED"))
			readBack := radix(store.RadixCommandData[float64]{
				Key:    key,
				Action: store.Read,
			})
			So(readBack, ShouldNotBeNil)
			So(*readBack, ShouldEqual, 42.0)
		})

		Convey("Changing the caller's write value does not mutate stored evidence", func() {
			val := 7.0
			radix(store.RadixCommandData[float64]{
				Key:    key,
				Value:  val,
				Action: store.Write,
			})
			val = 99.0
			readBack := radix(store.RadixCommandData[float64]{
				Key:    key,
				Action: store.Read,
			})
			So(readBack, ShouldNotBeNil)
			So(*readBack, ShouldEqual, 7.0)
		})
	})
}
