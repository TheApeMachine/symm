package manifold

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
)

/*
TestSampleConcurrentUpdates proves the BookSource read lease keeps Sample from
ranging Side.Levels while writers mutate the same map.
*/
func TestSampleConcurrentUpdates(t *testing.T) {
	source := newTestBookSource("BTC/USD")
	sampler := newBookSampler(source)
	tokenizer := NewTokenizer(pfluid.DefaultConfig())
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
			population, ok := sampler.Sample("BTC/USD", tokenizer)

			if !ok || len(population.batch.Particles) == 0 || population.midPrice <= 0 {
				t.Fatalf("final Sample mid=%v ok=%v particles=%d",
					population.midPrice, ok, len(population.batch.Particles))
			}

			return
		default:
			population, ok := sampler.Sample("BTC/USD", tokenizer)

			if !ok || len(population.batch.Particles) == 0 || population.midPrice <= 0 {
				t.Fatalf("Sample mid=%v ok=%v particles=%d",
					population.midPrice, ok, len(population.batch.Particles))
			}
		}
	}
}

/*
TestBookSampler_Sample proves a two-sided book yields owned touch scales and a
cold particle batch.
*/
func TestBookSampler_Sample(t *testing.T) {
	Convey("Given a sampled two-sided SDK book", t, func() {
		source := newTestBookSource("BTC/USD")
		sampler := newBookSampler(source)
		tokenizer := NewTokenizer(pfluid.DefaultConfig())
		population, ready := sampler.Sample("BTC/USD", tokenizer)

		So(ready, ShouldBeTrue)
		So(population.orderIDs, ShouldHaveLength, 2)
		So(population.batch.Particles, ShouldNotBeEmpty)
		So(population.reference, ShouldNotBeNil)
		So(population.reference.Sign(), ShouldEqual, 1)
		So(population.spread, ShouldBeGreaterThan, 0)
		So(population.buyCapacity.Sign(), ShouldEqual, 1)
		So(population.sellCapacity.Sign(), ShouldEqual, 1)

		Convey("Then removing the bid leaves the sample unreadied", func() {
			symbolBook := source.manager.GetBook("BTC/USD")
			source.apply(symbolBook, &book.UpdateOptions{
				Direction: book.Bid,
				ID:        "bid-1",
				Price:     decimal.NewFromFloat64(100),
				Quantity:  decimal.NewFromInt64(0),
				Timestamp: time.Unix(4, 0),
			})
			_, twoSided := sampler.Sample("BTC/USD", tokenizer)

			So(twoSided, ShouldBeFalse)
		})
	})
}

/*
BenchmarkBookSampler_Sample measures the leased sample path under a deep book.
*/
func BenchmarkBookSampler_Sample(b *testing.B) {
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
	tokenizer := NewTokenizer(pfluid.DefaultConfig())
	_, ready := sampler.Sample("BTC/USD", tokenizer)

	if !ready {
		b.Fatal("book sampler did not become ready")
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		population, sampled := sampler.Sample("BTC/USD", tokenizer)

		if !sampled || len(population.batch.Particles) == 0 {
			b.Fatal("book sampler lost the population")
		}
	}
}
