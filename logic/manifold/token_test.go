package manifold

import (
	"testing"

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
		orders := []restingOrder{
			{side: book.Bid, price: 99},
			{side: book.Ask, price: 101},
			{side: book.Bid, price: 99},
		}
		symbolIndex := uint32(2)

		Convey("It should pack content as sequence×side×symbol and site ω on the book circle", func() {
			batch := tokenizer.MakeBatch(orders, 100, 2.5, 0.75, symbolIndex)
			So(batch.Particles, ShouldHaveLength, len(orders))
			So(batch.ContentIDs, ShouldHaveLength, len(orders))

			for index, particle := range batch.Particles {
				So(particle.Velocity, ShouldResemble, pfluid.Vector{})
				So(particle.Mass, ShouldEqual, float32(1))
				So(particle.Omega, ShouldBeGreaterThanOrEqualTo, float32(-4))
				So(particle.Omega, ShouldBeLessThanOrEqualTo, float32(4))
				So(
					batch.ContentIDs[index],
					ShouldEqual,
					packContent(index, symbolIndex, orders[index].side),
				)

				if orders[index].side == book.Ask {
					So(particle.Energy, ShouldEqual, float32(0.75))
					So(particle.Heat, ShouldEqual, float32(0.75)*injectHeatFraction)
					continue
				}

				So(particle.Energy, ShouldEqual, float32(2.5))
				So(particle.Heat, ShouldEqual, float32(2.5)*injectHeatFraction)
			}

			cfg := tokenizer.config
			span := cfg.OmegaMax - cfg.OmegaMin
			So(batch.Particles[0].Omega, ShouldEqual, cfg.OmegaMin+(0.5)/3*span)
			So(batch.Particles[2].Omega, ShouldEqual, cfg.OmegaMin+(2.5)/3*span)
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

	for index := range orders {
		side := book.BookDirection(book.Bid)

		if index%2 == 0 {
			side = book.BookDirection(book.Ask)
		}

		orders[index] = restingOrder{
			side:  side,
			price: 100 + float64(index%7),
		}
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = tokenizer.MakeBatch(orders, 100, 1, 1, 3).Particles
	}
}
