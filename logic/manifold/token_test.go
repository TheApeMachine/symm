package manifold

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	mgrbook "github.com/theapemachine/api-go/v2/pkg/book"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
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

		Convey("It should conserve one unit of carrier mass and preserve liquidity shares", func() {
			So(err, ShouldBeNil)
			So(scaledErr, ShouldBeNil)
			So(reversedErr, ShouldBeNil)
			So(particles, ShouldHaveLength, len(orders))
			So(scaledParticles, ShouldHaveLength, len(scaledOrders))
			So(reversedParticles, ShouldHaveLength, len(orders))

			var totalMass float32

			for index, particle := range particles {
				totalMass += particle.Mass
				So(particle.Mass, ShouldAlmostEqual, scaledParticles[index].Mass, 1e-6)
				So(particle.Mass, ShouldBeGreaterThan, pfluid.MinimumPilotWaveMass)
				So(particle.Mass, ShouldBeLessThanOrEqualTo, float32(1))
				// Bids carry the buy-side excitation of 7 on top of the unit.
				So(particle.Energy, ShouldAlmostEqual, unitOscillatorEnergy*8, 1e-6)
				So(particle.Heat, ShouldEqual, float32(0))
				So(particle.Omega, ShouldAlmostEqual, scaledParticles[index].Omega, 1e-6)
				So(particle.Omega, ShouldAlmostEqual, reversedParticles[len(orders)-1-index].Omega, 1e-6)
			}

			So(totalMass, ShouldAlmostEqual, float32(1), 1e-6)
			So(particles[0].Omega, ShouldEqual, config.OmegaMin)
			So(particles[len(particles)-1].Omega, ShouldEqual, config.OmegaMax)
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
}
