package manifold

import (
	"math"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	. "github.com/smartystreets/goconvey/convey"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
)

func TestPackContent(t *testing.T) {
	Convey("Given sequence, symbol index, and side", t, func() {
		Convey("It should pack sequence in the top byte with side in the LSB", func() {
			bid := packContent(3, 5, book.Bid)
			ask := packContent(3, 5, book.Ask)
			So(bid>>24, ShouldEqual, uint32(3))
			So(ask>>24, ShouldEqual, uint32(3))
			So(bid&1, ShouldEqual, uint32(0))
			So(ask&1, ShouldEqual, uint32(1))
			So((bid>>1)&symbolIndexMask, ShouldEqual, uint32(5))
			So(bid, ShouldNotEqual, ask)
		})
	})
}

func TestTokenizer_MakeBatch(t *testing.T) {
	Convey("Given one L3 book sample with bid and ask resting orders", t, func() {
		tokenizer := NewTokenizer(pfluid.DefaultConfig())
		at := time.Unix(10, 0)
		orders := []restingOrder{
			{side: book.Bid, price: 99, quantity: 1, at: at},
			{side: book.Ask, price: 101, quantity: 4, at: at.Add(time.Second)},
			{side: book.Bid, price: 98, quantity: 2, at: at.Add(2 * time.Second)},
		}
		symbolIndex := uint32(2)

		Convey("It should place orders on log-price / log-size / age-rank axes", func() {
			batch := tokenizer.MakeBatch(orders, 100, 2.5, 0.75, symbolIndex)
			So(batch.Particles, ShouldHaveLength, len(orders))
			So(batch.ContentIDs, ShouldHaveLength, len(orders))

			grid := tokenizer.config.Grid
			extent := max(grid.X, grid.Y, grid.Z)
			spacing := float32(1.0 / float64(extent))
			zSpan := float32(grid.Z-1) * spacing

			for index, particle := range batch.Particles {
				So(particle.Velocity, ShouldResemble, pfluid.Vector{})
				So(particle.Mass, ShouldEqual, float32(1))
				So(
					batch.ContentIDs[index],
					ShouldEqual,
					packContent(index, symbolIndex, orders[index].side),
				)
			}

			// Oldest → Z=0, newest → Z=max. Prices span X; sizes span Y.
			So(float64(batch.Particles[0].Position.Z), ShouldAlmostEqual, 0, 1e-5)
			So(float64(batch.Particles[2].Position.Z), ShouldAlmostEqual, float64(zSpan), 1e-5)
			So(batch.Particles[0].Position.X, ShouldBeLessThan, batch.Particles[1].Position.X)
			So(batch.Particles[0].Position.Y, ShouldBeLessThan, batch.Particles[1].Position.Y)

			So(batch.Particles[1].Energy, ShouldEqual, float32(0.75))
			So(batch.Particles[0].Energy, ShouldEqual, float32(2.5))
		})
	})
}

func TestOrderAgeRanks(t *testing.T) {
	Convey("Given resting orders with distinct timestamps", t, func() {
		at := time.Unix(1, 0)
		orders := []restingOrder{
			{at: at.Add(2 * time.Second)},
			{at: at},
			{at: at.Add(time.Second)},
		}

		Convey("It should rank oldest as zero", func() {
			So(orderAgeRanks(orders), ShouldResemble, []int{2, 0, 1})
		})
	})
}

func TestMarketPosition(t *testing.T) {
	Convey("Given sample-relative market axes", t, func() {
		grid := pfluid.Grid{X: 64, Y: 64, Z: 64, Spacing: 1.0 / 64.0}
		spacing := float32(1.0 / 64.0)

		Convey("It should put extremes on opposite Z cells for age ranks", func() {
			oldest := marketPosition(
				100, 100, 1, 0, 3,
				-0.1, 0.1, 0, math.Log(4),
				grid, spacing,
			)
			newest := marketPosition(
				100, 100, 1, 2, 3,
				-0.1, 0.1, 0, math.Log(4),
				grid, spacing,
			)
			So(oldest.Z, ShouldEqual, float32(0))
			So(newest.Z, ShouldEqual, float32(63)*spacing)
		})
	})
}

func TestUniverseIndex(t *testing.T) {
	Convey("Given alphabetically sorted universe names", t, func() {
		universe := sortedUniverse([]string{"ETH/USD", "BTC/USD", "SOL/USD"})
		So(universe, ShouldResemble, []string{"BTC/USD", "ETH/USD", "SOL/USD"})

		index, ok := universeIndex(universe, "ETH/USD")
		So(ok, ShouldBeTrue)
		So(index, ShouldEqual, uint32(1))
	})
}

func TestPositionPhase(t *testing.T) {
	Convey("Given sequence indices on a book-sized beat", t, func() {
		Convey("It should keep phase in [0, 2π)", func() {
			phase := positionPhase(0, 3)
			So(phase, ShouldBeGreaterThanOrEqualTo, float32(0))
			So(float64(phase), ShouldBeLessThan, 2*3.141592653589793+1e-5)
			So(positionPhase(128, 3), ShouldNotEqual, positionPhase(0, 3))
		})
	})
}

func BenchmarkTokenizer_MakeBatch(b *testing.B) {
	tokenizer := NewTokenizer(pfluid.DefaultConfig())
	orders := make([]restingOrder, 64)
	at := time.Unix(1, 0)

	for index := range orders {
		side := book.BookDirection(book.Bid)

		if index%2 == 0 {
			side = book.BookDirection(book.Ask)
		}

		orders[index] = restingOrder{
			side:     side,
			price:    100 + float64(index%7),
			quantity: float64(1 + index%5),
			at:       at.Add(time.Duration(index) * time.Millisecond),
		}
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = tokenizer.MakeBatch(orders, 100, 1, 1, 3).Particles
	}
}
