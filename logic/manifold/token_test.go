package manifold

import (
	"fmt"
	"math"
	"testing"
	"time"

	mgrbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
	"github.com/theapemachine/symm/signal/compute"
)

func TestTokenizerNewBatch(t *testing.T) {
	Convey("Given a tokenizer with a deterministic market universe", t, func() {
		price := decimal.NewFromInt64(100)
		quantity := decimal.NewFromInt64(1)
		orders := []*mgrbook.Order{{
			ID:         "order",
			LimitPrice: price,
			Quantity:   quantity,
			Timestamp:  time.Unix(1, 0).UTC(),
		}}
		tokenizer := NewTokenizer(pfluid.DefaultConfig(), []string{"ETH/USD", "BTC/USD"})

		_, bitcoinContent, bitcoinErr := tokenizer.NewBatch(
			orders, nil, 100, 1, 1, "BTC/USD",
		)
		_, etherContent, etherErr := tokenizer.NewBatch(
			orders, nil, 100, 1, 1, "ETH/USD",
		)

		Convey("It should assign distinct stable content identities by symbol", func() {
			So(bitcoinErr, ShouldBeNil)
			So(etherErr, ShouldBeNil)
			So(bitcoinContent, ShouldResemble, []uint32{0})
			So(etherContent, ShouldResemble, []uint32{2})
		})

		Convey("It should reject a symbol outside that universe", func() {
			_, _, err := tokenizer.NewBatch(
				orders, nil, 100, 1, 1, "SOL/USD",
			)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "SOL/USD")
		})
	})

	Convey("Given visible orders expressed in different asset lot units", t, func() {
		config := pfluid.DefaultConfig()
		tokenizer := NewTokenizer(config, []string{"BTC/USD"})
		prices := []int64{99, 100, 101}
		quantities := []float64{51.869656, 186.96, 27632.9}
		orders := make([]*mgrbook.Order, 0, len(prices))
		scaledOrders := make([]*mgrbook.Order, 0, len(prices))

		for index, price := range prices {
			orders = append(orders, &mgrbook.Order{
				ID:         "order",
				LimitPrice: decimal.NewFromInt64(price),
				Quantity:   decimal.NewFromFloat64(quantities[index]),
				Timestamp:  time.Unix(int64(index+1), 0).UTC(),
			})
			scaledOrders = append(scaledOrders, &mgrbook.Order{
				ID:         "scaled-order",
				LimitPrice: decimal.NewFromInt64(price),
				Quantity:   decimal.NewFromFloat64(quantities[index] * 1000),
				Timestamp:  time.Unix(int64(index+1), 0).UTC(),
			})
		}

		particles, _, err := tokenizer.NewBatch(
			orders, nil, 100, 7, 11, "BTC/USD",
		)
		quietParticles, _, quietErr := tokenizer.NewBatch(
			orders, nil, 100, 0, 0, "BTC/USD",
		)
		scaledParticles, _, scaledErr := tokenizer.NewBatch(
			scaledOrders, nil, 100, 7, 11, "BTC/USD",
		)
		reversedParticles, _, reversedErr := tokenizer.NewBatch(
			[]*mgrbook.Order{orders[2], orders[1], orders[0]},
			nil,
			100,
			7,
			11,
			"BTC/USD",
		)

		Convey("It should inject one carrier mass unit per order independently of lot units", func() {
			So(err, ShouldBeNil)
			So(quietErr, ShouldBeNil)
			So(scaledErr, ShouldBeNil)
			So(reversedErr, ShouldBeNil)
			So(particles, ShouldHaveLength, len(orders))
			So(quietParticles, ShouldHaveLength, len(orders))
			So(scaledParticles, ShouldHaveLength, len(scaledOrders))
			So(reversedParticles, ShouldHaveLength, len(orders))

			var totalMass float32

			for index, particle := range particles {
				totalMass += particle.Mass
				So(particle.Mass, ShouldEqual, unitCarrierMass)
				So(scaledParticles[index].Mass, ShouldEqual, unitCarrierMass)
				// Bids carry the buy-side excitation of 7 on top of the unit.
				So(particle.Energy, ShouldAlmostEqual, unitOscillatorEnergy+7, 1e-6)
				So(quietParticles[index].Energy, ShouldEqual, unitOscillatorEnergy)
				So(particle.Heat, ShouldEqual, float32(0))
				So(particle.Omega, ShouldAlmostEqual, scaledParticles[index].Omega, 1e-6)
				So(particle.Omega, ShouldAlmostEqual, reversedParticles[len(orders)-1-index].Omega, 1e-6)
			}

			So(totalMass, ShouldEqual, float32(len(orders))*unitCarrierMass)
			So(particles[0].Omega, ShouldBeGreaterThanOrEqualTo, config.OmegaMin)
			So(particles[0].Omega, ShouldBeLessThan, particles[len(particles)-1].Omega)
			So(particles[len(particles)-1].Omega, ShouldBeLessThanOrEqualTo, config.OmegaMax)
		})

		Convey("It should remain finite through a coupled Metal advance", func() {
			var domain *pfluid.Domain
			domainErr := compute.WithMetalInit(func() error {
				created, err := pfluid.NewDomain(config)

				if err != nil {
					return err
				}

				domain = created
				return nil
			})
			So(domainErr, ShouldBeNil)
			Reset(func() { So(domain.Close(), ShouldBeNil) })

			contentIDs := make([]uint32, len(particles))

			for index := range contentIDs {
				contentIDs[index] = uint32(index + 1)
			}

			_, appendErr := domain.Append(particles, contentIDs)
			So(appendErr, ShouldBeNil)

			_, advanceErr := domain.Advance()
			So(advanceErr, ShouldBeNil)

			evolved, readErr := domain.ReadParticles(0, domain.ParticleCount())
			So(readErr, ShouldBeNil)

			for _, particle := range evolved {
				values := []float32{
					particle.Position.X,
					particle.Position.Y,
					particle.Position.Z,
					particle.Velocity.X,
					particle.Velocity.Y,
					particle.Velocity.Z,
					particle.Mass,
					particle.Heat,
					particle.Energy,
					particle.Phase,
					particle.Omega,
				}

				for _, value := range values {
					So(math.IsNaN(float64(value)), ShouldBeFalse)
					So(math.IsInf(float64(value), 0), ShouldBeFalse)
				}
			}
		})
	})

	Convey("Given a Hawkes intensity that cannot initialize binary32 heat", t, func() {
		orders := []*mgrbook.Order{{
			ID:         "order",
			LimitPrice: decimal.NewFromInt64(100),
			Quantity:   decimal.NewFromInt64(1),
			Timestamp:  time.Unix(1, 0).UTC(),
		}}
		tokenizer := NewTokenizer(pfluid.DefaultConfig(), []string{"BTC/USD"})

		Convey("It should reject the batch before converting it to binary32 heat", func() {
			for _, intensity := range []float64{math.Inf(1), math.MaxFloat64} {
				_, _, err := tokenizer.NewBatch(
					orders, nil, 100, intensity, 1, "BTC/USD",
				)

				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "representable as binary32")
			}
		})
	})

	Convey("Given a book whose order quantities span eight orders of magnitude", t, func() {
		config := pfluid.DefaultConfig()
		tokenizer := NewTokenizer(config, []string{"USDC/USD"})
		bidOrders := []*mgrbook.Order{
			{
				ID:         "large-bid",
				LimitPrice: decimal.NewFromFloat64(0.9999),
				Quantity:   decimal.NewFromFloat64(10_000_000),
				Timestamp:  time.Unix(1, 0).UTC(),
			},
			{
				ID:         "small-bid",
				LimitPrice: decimal.NewFromFloat64(0.9998),
				Quantity:   decimal.NewFromFloat64(1),
				Timestamp:  time.Unix(2, 0).UTC(),
			},
		}
		askOrders := []*mgrbook.Order{
			{
				ID:         "large-ask",
				LimitPrice: decimal.NewFromFloat64(1.0001),
				Quantity:   decimal.NewFromFloat64(10_000_000),
				Timestamp:  time.Unix(1, 0).UTC(),
			},
			{
				ID:         "small-ask",
				LimitPrice: decimal.NewFromFloat64(1.0002),
				Quantity:   decimal.NewFromFloat64(1),
				Timestamp:  time.Unix(2, 0).UTC(),
			},
		}

		particles, contentIDs, err := tokenizer.NewBatch(
			bidOrders, askOrders, 1, 2, 3, "USDC/USD",
		)

		Convey("It should preserve every order as a unit-mass carrier", func() {
			So(err, ShouldBeNil)
			So(particles, ShouldHaveLength, 4)
			So(contentIDs, ShouldResemble, []uint32{0, 0, 1, 1})

			var totalMass float32

			for index, particle := range particles {
				totalMass += particle.Mass
				So(particle.Mass, ShouldEqual, unitCarrierMass)
				So(particle.Heat, ShouldEqual, float32(0))

				if index < len(bidOrders) {
					So(particle.Energy, ShouldAlmostEqual, unitOscillatorEnergy+2, 1e-6)
				}

				if index >= len(bidOrders) {
					So(particle.Energy, ShouldAlmostEqual, unitOscillatorEnergy+3, 1e-6)
				}
			}

			So(totalMass, ShouldEqual, float32(4))
		})

		Convey("It should advance every carrier through the fluid domain", func() {
			var domain *pfluid.Domain
			domainErr := compute.WithMetalInit(func() error {
				created, err := pfluid.NewDomain(config)

				if err != nil {
					return err
				}

				domain = created
				return nil
			})
			So(domainErr, ShouldBeNil)
			Reset(func() { So(domain.Close(), ShouldBeNil) })

			_, appendErr := domain.Append(particles, contentIDs)
			So(appendErr, ShouldBeNil)

			_, advanceErr := domain.Advance()
			So(advanceErr, ShouldBeNil)
		})
	})
}

func BenchmarkTokenizerNewBatch(testingTB *testing.B) {
	const (
		orderCount    = 413
		bookSideCount = 2
	)

	config := pfluid.DefaultConfig()
	tokenizer := NewTokenizer(config, []string{"USDC/USD"})
	midPrice := 1.0
	dominantQuantity := float64(orderCount * orderCount * orderCount)
	bidOrders := make([]*mgrbook.Order, 0, orderCount/bookSideCount+1)
	askOrders := make([]*mgrbook.Order, 0, orderCount/bookSideCount+1)

	for index := range orderCount {
		distance := float64(index+1) / float64(orderCount+1) / float64(orderCount)
		quantity := 1.0

		if index < bookSideCount {
			quantity = dominantQuantity
		}

		order := &mgrbook.Order{
			ID:        fmt.Sprintf("order-%d", index),
			Quantity:  decimal.NewFromFloat64(quantity),
			Timestamp: time.Unix(int64(index+1), 0).UTC(),
		}

		if index%bookSideCount == 0 {
			order.LimitPrice = decimal.NewFromFloat64(midPrice - distance)
			bidOrders = append(bidOrders, order)

			continue
		}

		order.LimitPrice = decimal.NewFromFloat64(midPrice + distance)
		askOrders = append(askOrders, order)
	}

	testingTB.ReportAllocs()

	for testingTB.Loop() {
		particles, _, err := tokenizer.NewBatch(
			bidOrders, askOrders, midPrice, 2, 3, "USDC/USD",
		)

		if err != nil {
			testingTB.Fatal(err)
		}

		if len(particles) != orderCount {
			testingTB.Fatalf("expected %d particles, got %d", orderCount, len(particles))
		}

		for index, particle := range particles {
			if particle.Mass != unitCarrierMass {
				testingTB.Fatalf("particle %d has mass %g", index, particle.Mass)
			}
		}
	}
}
