package broker

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
)

func TestReadSnapshot(t *testing.T) {
	Convey("Given a tick carrying a multi-level book layer", t, func() {
		managed := newBookFixture(t, "BTC/USD", 0, 0)
		managed.Create("BTC/USD", 10)
		order := func(id string, price int64) kraken.Level3Order {
			return kraken.Level3Order{
				Event: "add", OrderID: id, LimitPrice: decimal.NewFromInt64(price),
				OrderQty: decimal.NewFromInt64(1), Timestamp: time.Unix(price, 0),
			}
		}
		So(managed.Update(&kraken.Level3{Data: []kraken.Level3Data{{
			Symbol: "BTC/USD",
			Bids:   []kraken.Level3Order{order("b1", 99), order("b2", 97), order("b3", 98)},
			Asks:   []kraken.Level3Order{order("a1", 102), order("a2", 101), order("a3", 103)},
		}}}), ShouldBeNil)

		value := data.NewValue("workspace", "tick", "aggregate").
			Append(managed.JSON("BTC/USD"), "book/BTC/USD")

		Convey("It decodes every level, best first", func() {
			snapshot, err := ReadSnapshot(value, "BTC/USD")
			So(err, ShouldBeNil)
			So(snapshot.Bids, ShouldHaveLength, 3)
			So(snapshot.Asks, ShouldHaveLength, 3)
			So(snapshot.Bids[0].Price.Float64(), ShouldEqual, 99)
			So(snapshot.Bids[2].Price.Float64(), ShouldEqual, 97)
			So(snapshot.Asks[0].Price.Float64(), ShouldEqual, 101)
			So(snapshot.Asks[2].Price.Float64(), ShouldEqual, 103)
		})

		Convey("A tick whose book is invalid is an error naming why, never a silent miss", func() {
			invalid := data.NewValue("workspace", "tick", "aggregate").
				Append([]byte(`{"kind":"checksum_mismatch"}`), "book_error/BTC/USD")
			snapshot, err := ReadSnapshot(invalid, "BTC/USD")
			So(snapshot, ShouldBeNil)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "checksum_mismatch")
		})

		Convey("An undecodable book layer is an error", func() {
			bad := data.NewValue("workspace", "tick", "aggregate").
				Append([]byte(`{"bids":7}`), "book/BTC/USD")
			_, err := ReadSnapshot(bad, "BTC/USD")
			So(err, ShouldNotBeNil)
		})
	})
}
