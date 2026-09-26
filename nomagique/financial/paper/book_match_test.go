package paper_test

import (
	"context"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/financial/paper"
)

func TestBookMatch(t *testing.T) {
	Convey("Trades are attributed to their market's existing reconciled touch", t, func() {
		client := paper.Book_ServerToClient(paper.NewBook(context.Background()))
		defer client.Release()
		_, err := replay(client, snapshotFrame("BTC/USD"))
		So(err, ShouldBeNil)
		for _, trade := range []struct {
			symbol, side    string
			price, quantity float64
			want            []float64
		}{
			{"BTC/USD", "sell", 99, .5, []float64{.5, .5, 0, 1, 0, 1, 1}},
			{"BTC/USD", "buy", 100, .25, []float64{.25, 0, .25, 0, 1, 1, 1}},
			{"BTC/USD", "buy", 99, .75, []float64{.75, 0, 0, 0, 0, 1, 1}},
			{"BTC/USD", "sell", 98, .25, []float64{0, 0, 0, 0, 0, 1, 1}},
			{"ETH/USD", "sell", 99, .5, []float64{0, 0, 0, 0, 0, 0, 0}},
		} {
			frame := fmt.Sprintf(`{"channel":"trade","data":{"symbol":%q,"side":%q,"price":%g,"qty":%g}}`, trade.symbol, trade.side, trade.price, trade.quantity)
			result, err := replay(client, []byte(frame))
			So(err, ShouldBeNil)
			So(result.Values[10:17], ShouldResemble, trade.want)
			So(result.Present[:10], ShouldResemble, make([]bool, 10))
			for _, present := range result.Present[10:17] {
				So(present, ShouldEqual, trade.symbol == "BTC/USD")
			}
		}
	})
}
