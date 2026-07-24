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

		Convey("It should emit universal-shaped cold particles with content ω", func() {
			batch := tokenizer.MakeBatch(orders, 100)
			So(batch.Particles, ShouldNotBeEmpty)
			So(batch.ContentIDs, ShouldHaveLength, len(batch.Particles))

			for index, particle := range batch.Particles {
				So(particle.Heat, ShouldEqual, float32(1e-4))
				So(particle.Energy, ShouldEqual, float32(1))
				So(particle.Velocity, ShouldResemble, pfluid.Vector{})
				So(particle.Mass, ShouldBeGreaterThan, float32(0))
				So(particle.Omega, ShouldBeGreaterThanOrEqualTo, float32(-4))
				So(particle.Omega, ShouldBeLessThanOrEqualTo, float32(4))
				So(batch.ContentIDs[index], ShouldBeLessThanOrEqualTo, uint32(255))
			}
		})

		Convey("It should compress identical content at the same relative sequence", func() {
			wrapped := make([]restingOrder, segmentLen+1)
			template := orders[0]

			for index := range wrapped {
				wrapped[index] = template
				wrapped[index].price = 100 + float64(index)
			}

			wrapped[0] = orders[0]
			wrapped[segmentLen] = orders[0]
			batch := tokenizer.MakeBatch(wrapped, 100)
			var merged float32

			for _, particle := range batch.Particles {
				if particle.Mass > 1 {
					merged = particle.Mass
				}
			}

			So(merged, ShouldEqual, float32(2))
			So(orderContent(orders[0], 100), ShouldNotEqual, orderContent(orders[1], 100))
		})
	})
}

func TestPositionPhase(t *testing.T) {
	Convey("Given sequence indices inside one segment", t, func() {
		Convey("It should keep phase in [0, 2π)", func() {
			phase := positionPhase(0)
			So(phase, ShouldBeGreaterThanOrEqualTo, float32(0))
			So(float64(phase), ShouldBeLessThan, 2*3.141592653589793+1e-5)
			So(positionPhase(128), ShouldNotEqual, positionPhase(0))
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
		_ = tokenizer.MakeBatch(orders, 100).Particles
	}
}
