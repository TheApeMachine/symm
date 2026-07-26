package kraken

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
)

/*
TestBookLevels proves book projection keeps decoded prices while deriving the
integer ticks the flow package consumes.
*/
func TestBookLevels(t *testing.T) {
	increment, err := decimal.NewFromString("0.01")

	if err != nil {
		t.Fatal(err)
	}

	bidPrice, err := decimal.NewFromString("100.02")

	if err != nil {
		t.Fatal(err)
	}

	askPrice, err := decimal.NewFromString("100.03")

	if err != nil {
		t.Fatal(err)
	}

	Convey("Given a stamped Kraken book row", t, func() {
		bids, asks, err := BookLevels(BookData{
			Symbol:         "SIM1/USD",
			PriceIncrement: increment,
			Bids:           []BookLevel{{Price: *bidPrice, Qty: 4}},
			Asks:           []BookLevel{{Price: *askPrice, Qty: 5}},
			Timestamp:      time.Unix(1, 0).UTC(),
		})

		Convey("It should project exact ticks for both sides", func() {
			So(err, ShouldBeNil)
			So(bids, ShouldHaveLength, 1)
			So(asks, ShouldHaveLength, 1)
			So(bids[0].Ticks, ShouldEqual, int64(10002))
			So(asks[0].Ticks, ShouldEqual, int64(10003))
		})
	})
}

/*
BenchmarkBookLevels measures one direct book row projection into flow levels.
*/
func BenchmarkBookLevels(b *testing.B) {
	increment, err := decimal.NewFromString("0.01")

	if err != nil {
		b.Fatal(err)
	}

	bidPrice, err := decimal.NewFromString("100.02")

	if err != nil {
		b.Fatal(err)
	}

	askPrice, err := decimal.NewFromString("100.03")

	if err != nil {
		b.Fatal(err)
	}

	row := BookData{
		Symbol:         "SIM1/USD",
		PriceIncrement: increment,
		Bids:           []BookLevel{{Price: *bidPrice, Qty: 4}},
		Asks:           []BookLevel{{Price: *askPrice, Qty: 5}},
		Timestamp:      time.Unix(1, 0).UTC(),
	}

	b.ReportAllocs()

	for b.Loop() {
		bids, asks, err := BookLevels(row)

		if err != nil {
			b.Fatal(err)
		}

		if len(bids) != 1 || len(asks) != 1 {
			b.Fatal("book projection lost a side")
		}
	}
}
