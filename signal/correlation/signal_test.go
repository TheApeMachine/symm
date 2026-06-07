package correlation

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/bus"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestNewSignal(t *testing.T) {
	Convey("Given a qpool", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		Convey("It should wire herd categories", func() {
			So(signal.categories["systemic_herd"], ShouldEqual, types.CategorySystemicHerd)
			So(signal.categories["decoupled_alpha"], ShouldEqual, types.CategoryDecoupledAlpha)
		})
	})
}

func TestFingerprintColdHist(t *testing.T) {
	Convey("Given an all-zero return ring", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		state := &symbolState{}

		Convey("It should degenerate to all-ones under the >= 0 tie-break", func() {
			So(signal.fingerprint(state), ShouldEqual, ^uint64(0))
		})
	})
}

func TestProcessColdStart(t *testing.T) {
	Convey("Given a correlation signal before the ring is full", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		measurements := signal.broadcasts["measurements"].Subscribe("test:cold", 64)

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

		Convey("It should not publish false herd readings", func() {
			waitCtx, waitCancel := context.WithTimeout(ctx, 50*time.Millisecond)
			defer waitCancel()

			if value, err := bus.PollFor(waitCtx, measurements); err == nil {
				t.Fatalf("unexpected measurement during warm-up: %+v", value.Value)
			}
		})
	})
}

func TestProcess(t *testing.T) {
	Convey("Given a correlation signal with a measurements subscriber", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		measurements := signal.broadcasts["measurements"].Subscribe("test:correlation", 64)

		prices := map[string]float64{
			"BTC/EUR": 100,
			"ETH/EUR": 50,
			"SOL/EUR": 25,
		}

		Convey("When the cross-section moves together after warm-up", func() {
			for range gridBars + 1 {
				signal.process(prices)
				prices = map[string]float64{
					"BTC/EUR": prices["BTC/EUR"] * 1.01,
					"ETH/EUR": prices["ETH/EUR"] * 1.01,
					"SOL/EUR": prices["SOL/EUR"] * 1.01,
				}
			}

			waitCtx, waitCancel := context.WithTimeout(ctx, time.Second)
			defer waitCancel()

			value, err := bus.PollFor(waitCtx, measurements)
			if err != nil {
				t.Fatal("timed out waiting for correlation measurement")
			}

			measurement, _ := value.Value.(types.Measurement)

			Convey("It publishes a herd-behavior reading", func() {
				So(measurement.Source, ShouldEqual, types.SourceCorrelation)
				So(measurement.Symbol, ShouldNotBeEmpty)
				So(measurement.SNR, ShouldBeGreaterThanOrEqualTo, 0)
				So(measurement.Strength, ShouldBeGreaterThan, 0)
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
	pool := qpool.NewQ[any](ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	signal := NewSignal(ctx, pool)
	defer signal.Close()

	signal.broadcasts["measurements"].Subscribe("bench:correlation", 1024)

	prices := map[string]float64{"BTC/EUR": 100, "ETH/EUR": 50, "SOL/EUR": 25}

	for range gridBars + 1 {
		signal.process(prices)
		prices = map[string]float64{
			"BTC/EUR": prices["BTC/EUR"] * 1.01,
			"ETH/EUR": prices["ETH/EUR"] * 1.01,
			"SOL/EUR": prices["SOL/EUR"] * 1.01,
		}
	}

	b.ReportAllocs()

	for b.Loop() {
		signal.process(prices)
		prices["BTC/EUR"] *= 1.01
		prices["ETH/EUR"] *= 1.01
		prices["SOL/EUR"] *= 1.01
	}
}
