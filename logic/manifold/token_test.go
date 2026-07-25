package manifold

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/book"
	. "github.com/smartystreets/goconvey/convey"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
)

func TestTokenizer_MakeBatch(t *testing.T) {
	Convey("Given one L3 book sample with bid and ask resting orders", t, func() {
		tokenizer := NewTokenizer(pfluid.DefaultConfig())
		orders := []restingOrder{
			{side: book.Bid, price: 99},
			{side: book.Ask, price: 101},
			{side: book.Bid, price: 99},
		}

		Convey("It should place one content site per order on the book circle", func() {
			batch := tokenizer.MakeBatch(orders, 100, 2.5, 0.75)
			So(batch.Particles, ShouldHaveLength, len(orders))
			So(batch.ContentIDs, ShouldHaveLength, len(orders))

			for index, particle := range batch.Particles {
				So(particle.Velocity, ShouldResemble, pfluid.Vector{})
				So(particle.Mass, ShouldEqual, float32(1))
				So(particle.Omega, ShouldBeGreaterThanOrEqualTo, float32(-4))
				So(particle.Omega, ShouldBeLessThanOrEqualTo, float32(4))
				So(batch.ContentIDs[index], ShouldEqual, uint32(index))

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
		_ = tokenizer.MakeBatch(orders, 100, 1, 1).Particles
	}
}
