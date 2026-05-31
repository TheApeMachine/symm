package correlation

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestNewSignal(t *testing.T) {
	Convey("Given a qpool", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		Convey("It should wire herd categories", func() {
			So(signal.categories["systemic_herd"], ShouldEqual, perspectives.CategorySystemicHerd)
			So(signal.categories["decoupled_alpha"], ShouldEqual, perspectives.CategoryDecoupledAlpha)
		})
	})
}

func TestProcess(t *testing.T) {
	Convey("Given a correlation signal with a measurements subscriber", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		measurements := signal.broadcasts["measurements"].Subscribe("test:correlation", 64)

		Convey("When the cross-section moves together across two windows", func() {
			signal.process(map[string]float64{
				"BTC/EUR": 100,
				"ETH/EUR": 50,
				"SOL/EUR": 25,
			})
			signal.process(map[string]float64{
				"BTC/EUR": 101,
				"ETH/EUR": 50.5,
				"SOL/EUR": 25.25,
			})

			var measurement perspectives.Measurement
			received := false
			deadline := time.After(time.Second)

			for !received {
				select {
				case value := <-measurements.Incoming:
					reading, ok := value.Value.(perspectives.Measurement)

					if ok {
						measurement = reading
						received = true
					}
				case <-deadline:
					t.Fatal("timed out waiting for correlation measurement")
				}
			}

			Convey("It publishes a herd-behavior reading", func() {
				So(measurement.Source, ShouldEqual, perspectives.SourceCorrelation)
				So(measurement.Symbol, ShouldNotBeEmpty)
				So(measurement.SNR, ShouldBeGreaterThanOrEqualTo, 0)
			})
		})
	})
}

func TestMarketMode(t *testing.T) {
	Convey("Given fingerprints from three coins", t, func() {
		signal := &Signal{}
		active := []live{
			{sig: 0b101},
			{sig: 0b111},
			{sig: 0b110},
		}

		mode := signal.marketMode(active)

		Convey("It should vote the majority bit pattern", func() {
			So(mode, ShouldEqual, uint64(0b111))
		})
	})
}

func BenchmarkProcess(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	signal := NewSignal(ctx, pool)
	defer signal.Close()

	signal.broadcasts["measurements"].Subscribe("bench:correlation", 1024)

	seed := map[string]float64{"BTC/EUR": 100, "ETH/EUR": 50, "SOL/EUR": 25}
	tick := map[string]float64{"BTC/EUR": 101, "ETH/EUR": 50.5, "SOL/EUR": 25.25}

	signal.process(seed)

	b.ReportAllocs()

	for b.Loop() {
		signal.process(tick)
	}
}
