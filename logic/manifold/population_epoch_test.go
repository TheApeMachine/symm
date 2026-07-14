package manifold

import (
	"testing"
	"time"

	"github.com/theapemachine/symm/kraken"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPopulationBeginEpoch(t *testing.T) {
	Convey("Given multiple authoritative rows accumulated in one population", t, func() {
		population := NewPopulation("BTC/USD", nil, 10)
		population.Apply(kraken.Level3Data{
			Type:      "snapshot",
			Timestamp: time.Unix(1, 0),
			Bids: []kraken.Level3Order{{
				OrderID: "bid-1", LimitPrice: 99, OrderQty: 1,
			}},
			Asks: []kraken.Level3Order{{
				OrderID: "ask-1", LimitPrice: 101, OrderQty: 1,
			}},
		})
		population.Apply(kraken.Level3Data{
			Type:      "update",
			Timestamp: time.Unix(2, 0),
			Bids: []kraken.Level3Order{{
				Event: "modify", OrderID: "bid-1", LimitPrice: 99, OrderQty: 2,
			}},
		})

		Convey("When the accumulated population begins one field epoch", func() {
			first := population.BeginEpoch()
			second := population.BeginEpoch()

			Convey("Then epoch identity follows field boundaries rather than raw row count", func() {
				So(first, ShouldEqual, uint64(1))
				So(second, ShouldEqual, uint64(2))
				So(population.Epoch(), ShouldEqual, uint64(2))
			})
		})
	})
}
