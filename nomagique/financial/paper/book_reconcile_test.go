package paper_test

import (
	"context"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/financial/paper"
	marketfixture "github.com/theapemachine/symm/tests/market"
	"testing"
)

func TestBookReconcileL3(t *testing.T) {
	Convey("The canonical SDK aggregate equals its exact order queue through mixed precision mutations", t, func() {
		client := paper.Book_ServerToClient(paper.NewBook(context.Background()))
		defer client.Release()
		first := resting{"99", "1", "2026-09-23T09:00:00Z"}
		second := resting{"99", "0.25", "2026-09-23T09:00:01Z"}
		deeper := resting{"98", "3", "2026-09-23T09:00:02Z"}
		state := [2][]resting{{first, second, deeper}, asks}
		result, err := replay(client, marketfixture.Level3Frame("snapshot", "BTC/USD", [2][]resting{}, state, ""))
		So(err, ShouldBeNil)
		So(result.Bids, ShouldResemble, [][2]string{{"99", "1.25"}, {"98", "3"}})
		assertBookQueueTotals(client)
		first.Quantity = "0.1"
		result, err = replay(client, marketfixture.Level3Frame("update", "BTC/USD", state, [2][]resting{{first}, nil}, "modify"))
		So(err, ShouldBeNil)
		So(result.Bids, ShouldResemble, [][2]string{{"99", "0.35"}, {"98", "3"}})
		assertBookQueueTotals(client)
		state[0] = []resting{first, second, deeper}
		second.Quantity = "2"
		result, err = replay(client, marketfixture.Level3Frame("update", "BTC/USD", state, [2][]resting{{second}, nil}, "modify"))
		So(err, ShouldBeNil)
		So(result.Bids, ShouldResemble, [][2]string{{"99", "2.10"}, {"98", "3"}})
		assertBookQueueTotals(client)
		state[0] = []resting{first, second, deeper}
		result, err = replay(client, marketfixture.Level3Frame("update", "BTC/USD", state, [2][]resting{{first}, nil}, "delete"))
		So(err, ShouldBeNil)
		So(result.Bids, ShouldResemble, [][2]string{{"99", "2.00"}, {"98", "3"}})
		assertBookQueueTotals(client)
		state[0] = []resting{second, deeper}
		result, err = replay(client, marketfixture.Level3Frame("update", "BTC/USD", state, [2][]resting{{second}, nil}, "delete"))
		So(err, ShouldBeNil)
		So(result.Bids, ShouldResemble, [][2]string{{"98", "3"}})
		assertBookQueueTotals(client)
		first.Quantity = "0.0005"
		state[0] = []resting{deeper}
		result, err = replay(client, marketfixture.Level3Frame("update", "BTC/USD", state, [2][]resting{{first}, nil}, "add"))
		So(err, ShouldBeNil)
		So(result.Bids, ShouldResemble, [][2]string{{"99", "0.0005"}, {"98", "3"}})
		assertBookQueueTotals(client)
	})
}

func assertBookQueueTotals(client paper.Book) {
	future, release := client.Done(context.Background(), nil)
	defer release()
	result, err := future.Struct()
	So(err, ShouldBeNil)
	market, err := result.Market()
	So(err, ShouldBeNil)
	orders, err := market.Orders()
	So(err, ShouldBeNil)
	sums := map[bool]map[string]*decimal.Decimal{true: {}, false: {}}
	for index := range orders.Len() {
		order := orders.At(index)
		price, err := order.Price()
		So(err, ShouldBeNil)
		quantity, err := order.Quantity()
		So(err, ShouldBeNil)
		value, err := decimal.NewFromString(quantity)
		So(err, ShouldBeNil)
		total := sums[order.Bid()][price]
		if total == nil {
			sums[order.Bid()][price] = value
			continue
		}
		sums[order.Bid()][price] = total.SetScale(max(total.GetScale(), value.GetScale())).Add(value)
	}
	for _, bid := range []bool{true, false} {
		levels, err := market.Asks()
		if bid {
			levels, err = market.Bids()
		}
		So(err, ShouldBeNil)
		So(levels.Len(), ShouldEqual, len(sums[bid]))
		for index := range levels.Len() {
			level := levels.At(index)
			price, err := level.Price()
			So(err, ShouldBeNil)
			quantity, err := level.Quantity()
			So(err, ShouldBeNil)
			value, err := decimal.NewFromString(quantity)
			So(err, ShouldBeNil)
			So(value.Cmp(sums[bid][price]), ShouldEqual, 0)
		}
	}
}

func BenchmarkBookReconcileL3(b *testing.B) {
	ctx := context.Background()
	client := paper.Book_ServerToClient(paper.NewBook(ctx))
	defer client.Release()
	first := resting{"99", "1", "2026-09-23T09:00:00Z"}
	second := resting{"99", "0.25", "2026-09-23T09:00:01Z"}
	initial := [2][]resting{{first, second}, asks}
	changed := first
	changed.Quantity = "0.1"
	frames := [][]byte{
		marketfixture.Level3Frame("snapshot", "BTC/USD", [2][]resting{}, initial, ""),
		marketfixture.Level3Frame("update", "BTC/USD", initial, [2][]resting{{changed}, nil}, "modify"),
		marketfixture.Level3Frame("update", "BTC/USD", [2][]resting{{changed, second}, asks}, [2][]resting{{second}, nil}, "delete"),
	}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := range b.N {
		if err := client.Write(ctx, func(args paper.Book_write_Params) error {
			args.SetDepth(10)
			arrivals, err := args.NewFrame(1)
			if err != nil {
				return err
			}
			return arrivals.Set(0, frames[iteration%len(frames)])
		}); err != nil {
			b.Fatal(err)
		}
		if err := client.WaitStreaming(); err != nil {
			b.Fatal(err)
		}
		future, release := client.Done(ctx, nil)
		result, err := future.Struct()
		if err != nil {
			b.Fatal(err)
		}
		present, err := result.Present()
		if err != nil || !present.At(0) {
			b.Fatalf("reconciled native bid missing: %v", err)
		}
		release()
	}
}
