package paper_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/financial/paper"
	marketfixture "github.com/theapemachine/symm/tests/market"
)

func TestBookTouch(t *testing.T) {
	Convey("Reconciled L3 brackets preserve prior touch and matched executions", t, func() {
		client := paper.Book_ServerToClient(paper.NewBook(context.Background()))
		defer client.Release()
		initial, err := replay(client, snapshotFrame("BTC/USD"))
		So(err, ShouldBeNil)
		So(initial.Present[17:], ShouldResemble, make([]bool, 12))
		_, err = replay(client, []byte(`{"channel":"trade","data":{"symbol":"BTC/USD","side":"sell","price":99,"qty":0.5}}`))
		So(err, ShouldBeNil)
		reduced := bids[0]
		reduced.Quantity = "0.25"
		result, err := replay(client, marketfixture.Level3Frame("update", "BTC/USD", [2][]resting{bids, asks}, [2][]resting{{reduced}, nil}, "modify"))
		So(err, ShouldBeNil)
		So(result.Values[17:], ShouldResemble, []float64{1, 1, .5, 0, .25, 1, 0, 0, 1, 1, 99, 100})

		Convey("A retreat reports its remaining displayed quantity independently of price distance", func() {
			_, err = replay(client, []byte(`{"channel":"trade","data":{"symbol":"BTC/USD","side":"sell","price":99,"qty":0.1}}`))
			So(err, ShouldBeNil)
			result, err = replay(client, marketfixture.Level3Frame("update", "BTC/USD", [2][]resting{{reduced, bids[1]}, asks}, [2][]resting{{reduced}, nil}, "delete"))
			So(err, ShouldBeNil)
			So(result.Values[17], ShouldEqual, .25)
			So(result.Values[19], ShouldEqual, .1)
			So(result.Values[23], ShouldEqual, 1)
			So(result.Values[25], ShouldEqual, 0)
			So((result.Values[17]-result.Values[19])*result.Values[23], ShouldAlmostEqual, .15)
			So(result.Values[27], ShouldEqual, 99)
			So(result.Values[0], ShouldEqual, 98)
		})
		Convey("Executions exceeding the previous touch cannot be attributed beyond it", func() {
			_, err = replay(client, []byte(`{"channel":"trade","data":{"symbol":"BTC/USD","side":"buy","price":100,"qty":1.5}}`))
			So(err, ShouldBeNil)
			changedAsk := asks[0]
			changedAsk.Quantity = "0.5"
			result, err = replay(client, marketfixture.Level3Frame("update", "BTC/USD", [2][]resting{{reduced, bids[1]}, asks}, [2][]resting{nil, {changedAsk}}, "modify"))
			So(err, ShouldBeNil)
			So(result.Values[20], ShouldEqual, 1)
			So(result.Values[22], ShouldEqual, .5)
			So(result.Values[19], ShouldEqual, 0)
		})
		Convey("A fresh snapshot clears the previous bracket and does not invent prior evidence", func() {
			result, err = replay(client, snapshotFrame("BTC/USD"))
			So(err, ShouldBeNil)
			So(result.Present[17:], ShouldResemble, make([]bool, 12))
			result, err = replay(client, deepBidFrame("add"))
			So(err, ShouldBeNil)
			So(result.Values[19:21], ShouldResemble, []float64{0, 0})
		})
	})
}
