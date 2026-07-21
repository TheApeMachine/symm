package manifold

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
)

/*
TestOrdersForSymbolConcurrentUpdates proves the BookSource read lease keeps
ordersForSymbol from ranging Side.Levels while writers mutate the same map.
*/
func TestOrdersForSymbolConcurrentUpdates(t *testing.T) {
	source := newTestBookSource("BTC/USD")
	sampler := newBookSampler(source)
	symbolBook := source.manager.GetBook("BTC/USD")
	done := make(chan struct{})
	var wait sync.WaitGroup

	wait.Add(1)

	go func() {
		defer wait.Done()

		at := time.Unix(2, 0)

		for index := 0; index < 2_000; index++ {
			price := 100.0 + float64(index%20)*0.01
			source.apply(symbolBook, &book.UpdateOptions{
				Direction: book.Bid,
				ID:        "bid-race",
				Price:     decimal.NewFromFloat64(price),
				Quantity:  decimal.NewFromFloat64(1),
				Timestamp: at.Add(time.Duration(index) * time.Millisecond),
			})
		}

		close(done)
	}()

	for {
		select {
		case <-done:
			wait.Wait()
			orders, mid, ok := sampler.Orders("BTC/USD")

			if !ok || len(orders) == 0 || mid <= 0 {
				t.Fatalf("final ordersForSymbol = %d mid=%v ok=%v", len(orders), mid, ok)
			}

			return
		default:
			orders, mid, ok := sampler.Orders("BTC/USD")

			if !ok || len(orders) == 0 || mid <= 0 {
				t.Fatalf("ordersForSymbol = %d mid=%v ok=%v", len(orders), mid, ok)
			}
		}
	}
}

/*
TestBookSampler_Orders proves unchanged SDK decimals are reused while replaced
quantities refresh and departed identities leave the bounded cache.
*/
func TestBookSampler_Orders(t *testing.T) {
	Convey("Given a sampled two-sided SDK book", t, func() {
		source := newTestBookSource("BTC/USD")
		sampler := newBookSampler(source)
		symbolBook := source.manager.GetBook("BTC/USD")
		first, _, ready := sampler.Orders("BTC/USD")

		So(ready, ShouldBeTrue)
		So(first, ShouldHaveLength, 2)
		firstBid := first[0]

		Convey("Then an unchanged sample reuses its converted money", func() {
			second, _, secondReady := sampler.Orders("BTC/USD")

			So(secondReady, ShouldBeTrue)
			So(second[0].priceMoney, ShouldEqual, firstBid.priceMoney)
			So(second[0].quantityMoney, ShouldEqual, firstBid.quantityMoney)
		})

		Convey("Then a modified quantity refreshes only that order", func() {
			source.apply(symbolBook, &book.UpdateOptions{
				Direction: book.Bid,
				ID:        "bid-1",
				Price:     decimal.NewFromFloat64(100),
				Quantity:  decimal.NewFromFloat64(4),
				Timestamp: time.Unix(4, 0),
			})
			third, _, thirdReady := sampler.Orders("BTC/USD")

			So(thirdReady, ShouldBeTrue)
			So(third[0].quantity, ShouldEqual, 4.0)
			So(third[0].quantityMoney, ShouldNotEqual, firstBid.quantityMoney)
			So(sampler.samples["BTC/USD"].cache, ShouldHaveLength, 2)
		})

		Convey("Then a departed order is removed from the retained cache", func() {
			source.apply(symbolBook, &book.UpdateOptions{
				Direction: book.Bid,
				ID:        "bid-1",
				Price:     decimal.NewFromFloat64(100),
				Quantity:  decimal.NewFromInt64(0),
				Timestamp: time.Unix(4, 0),
			})
			_, _, twoSided := sampler.Orders("BTC/USD")
			_, retained := sampler.samples["BTC/USD"].cache["bid-1"]

			So(twoSided, ShouldBeFalse)
			So(retained, ShouldBeFalse)
			So(sampler.samples["BTC/USD"].cache, ShouldHaveLength, 1)
		})
	})
}

/*
BenchmarkBookSampler_Orders measures the unchanged-book tick path that formerly
recreated every Decimal big.Rat and exact-money copy on every analyzer update.
*/
func BenchmarkBookSampler_Orders(b *testing.B) {
	source := newTestBookSource("BTC/USD")
	symbolBook := source.manager.GetBook("BTC/USD")
	at := time.Unix(2, 0)

	for level := range 100 {
		source.apply(symbolBook, &book.UpdateOptions{
			Direction: book.Bid,
			ID:        "bid-depth-" + strconv.Itoa(level),
			Price:     decimal.NewFromFloat64(99 - float64(level)/100),
			Quantity:  decimal.NewFromFloat64(float64(level + 1)),
			Timestamp: at.Add(time.Duration(level) * time.Millisecond),
		})
		source.apply(symbolBook, &book.UpdateOptions{
			Direction: book.Ask,
			ID:        "ask-depth-" + strconv.Itoa(level),
			Price:     decimal.NewFromFloat64(102 + float64(level)/100),
			Quantity:  decimal.NewFromFloat64(float64(level + 1)),
			Timestamp: at.Add(time.Duration(level) * time.Millisecond),
		})
	}

	sampler := newBookSampler(source)
	_, _, ready := sampler.Orders("BTC/USD")

	if !ready {
		b.Fatal("book sampler did not become ready")
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		orders, _, sampled := sampler.Orders("BTC/USD")

		if !sampled || len(orders) == 0 {
			b.Fatal("book sampler lost the unchanged population")
		}
	}
}
