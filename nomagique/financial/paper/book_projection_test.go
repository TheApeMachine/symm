package paper_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/financial/paper"
)

func TestBookProject(t *testing.T) {
	Convey("Native projections distinguish maintained liquidity from arriving mutations", t, func() {
		client := paper.Book_ServerToClient(paper.NewBook(context.Background()))
		defer client.Release()
		first, err := replay(client, snapshotFrame("BTC/USD"))
		So(err, ShouldBeNil)
		So(first.Values[:10], ShouldResemble, []float64{99, 100, 1, 1, 4, 3, 393, 302, 2, 2})
		So(first.Present[:10], ShouldResemble, []bool{true, true, true, true, true, true, true, true, true, true})

		Convey("A one-sided deep addition leaves both best prices unchanged", func() {
			added, err := replay(client, deepBidFrame("add"))
			So(err, ShouldBeNil)
			So(added.Values[:10], ShouldResemble, []float64{99, 100, 1, 1, 5, 3, 90, 0, 1, 0})

			Convey("Deleting that order has no displayed notional and removes its quantity", func() {
				deleted, err := replay(client, deepBidFrame("delete"))
				So(err, ShouldBeNil)
				So(deleted.Values[:10], ShouldResemble, []float64{99, 100, 1, 1, 4, 3, 0, 0, 1, 0})
			})
		})

		Convey("A checksum failure invalidates touch and depth without erasing wire facts", func() {
			lost, err := replay(client, []byte(`{"channel":"level3","type":"update","data":{"symbol":"BTC/USD","checksum":1,"bids":[],"asks":[]}}`))
			So(err, ShouldBeNil)
			So(lost.Present[:10], ShouldResemble, []bool{false, false, false, false, false, false, true, true, true, true})
			recovered, err := replay(client, snapshotFrame("BTC/USD"))
			So(err, ShouldBeNil)
			So(recovered.Values[:10], ShouldResemble, first.Values[:10])
		})
	})
}

func TestBookFlow(t *testing.T) {
	Convey("Malformed wire mutations cannot become zero-valued observations", t, func() {
		for _, mutation := range []string{
			`{"event":"invented","limit_price":1,"order_qty":2}`,
			`{"event":"add","order_qty":2}`,
			`{"event":"modify","limit_price":1,"order_qty":-2}`,
		} {
			client := paper.Book_ServerToClient(paper.NewBook(context.Background()))
			_, err := replay(client, []byte(`{"channel":"level3","type":"update","data":{"symbol":"BTC/USD","bids":[`+mutation+`],"asks":[]}}`))
			So(err, ShouldNotBeNil)
			client.Release()
		}
	})
}
