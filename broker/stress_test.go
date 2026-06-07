package broker

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestStressCacheIngestMeasurement(t *testing.T) {
	Convey("Given a stress cache", t, func() {
		cache := NewStressCache(context.Background(), nil)

		Convey("It should record toxicity readings", func() {
			cache.ingestMeasurement(types.Measurement{
				Symbol:   "BTC/EUR",
				Source:   types.SourceToxicity,
				Category: types.CategoryToxicBluff,
				SNR:      1.2,
			})

			stress := cache.Snapshot("BTC/EUR")

			So(stress.ToxicityCategory, ShouldEqual, types.CategoryToxicBluff)
			So(stress.ToxicitySNR, ShouldAlmostEqual, 1.2, 1e-9)
		})

		Convey("It should record fluid readings", func() {
			cache.ingestMeasurement(types.Measurement{
				Symbol:   "BTC/EUR",
				Source:   types.SourceFluid,
				Category: types.CategoryTurbulent,
				SNR:      0.8,
			})

			stress := cache.Snapshot("BTC/EUR")

			So(stress.FluidCategory, ShouldEqual, types.CategoryTurbulent)
			So(stress.FluidSNR, ShouldAlmostEqual, 0.8, 1e-9)
		})

		Convey("It should record sentiment readings", func() {
			cache.ingestMeasurement(types.Measurement{
				Symbol:   "BTC/EUR",
				Source:   types.SourceSentiment,
				Category: types.CategorySystemicSlump,
				SNR:      2.1,
			})

			stress := cache.Snapshot("BTC/EUR")

			So(stress.SentimentCategory, ShouldEqual, types.CategorySystemicSlump)
			So(stress.SentimentSNR, ShouldAlmostEqual, 2.1, 1e-9)
		})

		Convey("It should ignore unknown sources", func() {
			cache.ingestMeasurement(types.Measurement{
				Symbol: "ETH/EUR",
				Source: types.SourceExhaustion,
				SNR:    9,
			})

			So(cache.Snapshot("ETH/EUR"), ShouldResemble, SymbolStress{})
		})

		Convey("It should ignore empty symbols", func() {
			cache.ingestMeasurement(types.Measurement{
				Source: types.SourceHawkes,
				SNR:    1,
			})

			So(cache.Snapshot(""), ShouldResemble, SymbolStress{})
		})
	})
}

func TestEnsureStressCacheConstructsInstance(t *testing.T) {
	Convey("Given a pool", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		pool := qpool.NewQ[any](ctx, 1, 4, nil)

		defer func() {
			cancel()
			pool.Close()
		}()

		Convey("EnsureStressCache should return a started cache", func() {
			cache := EnsureStressCache(ctx, pool)

			So(cache, ShouldNotBeNil)
			So(cache.started.Load(), ShouldBeTrue)
		})
	})
}

func TestStressCacheBroadcast(t *testing.T) {
	Convey("Given a stress cache subscribed to measurements", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 4, nil)
		defer pool.Close()

		measurements := pool.CreateBroadcastGroup("measurements", 0)
		cache := NewStressCache(ctx, pool)
		cache.Start(pool)

		measurement := types.Measurement{
			Symbol:   "BTC/EUR",
			Source:   types.SourceToxicity,
			Category: types.CategoryToxicBluff,
			SNR:      1.5,
		}

		deadline := time.Now().Add(2 * time.Second)
		var stress SymbolStress

		for time.Now().Before(deadline) {
			measurements.Send(&qpool.QValue[any]{Value: measurement})
			stress = cache.Snapshot("BTC/EUR")

			if stress.ToxicityCategory == types.CategoryToxicBluff {
				break
			}

			time.Sleep(5 * time.Millisecond)
		}

		Convey("It should ingest broadcast measurements", func() {
			So(stress.ToxicityCategory, ShouldEqual, types.CategoryToxicBluff)
			So(stress.ToxicitySNR, ShouldAlmostEqual, 1.5, 1e-9)
		})
	})
}

func BenchmarkStressCacheSnapshot(b *testing.B) {
	cache := NewStressCache(context.Background(), nil)
	cache.InstallStressForTest("BTC/EUR", SymbolStress{
		ToxicityCategory: types.CategoryToxicBluff,
		ToxicitySNR:      1,
	})

	for b.Loop() {
		_ = cache.Snapshot("BTC/EUR")
	}
}

func BenchmarkStressCacheIngestMeasurement(b *testing.B) {
	cache := NewStressCache(context.Background(), nil)
	measurement := types.Measurement{
		Symbol:   "BTC/EUR",
		Source:   types.SourceHawkes,
		Category: types.CategorySaturation,
		SNR:      2.5,
	}

	for b.Loop() {
		cache.ingestMeasurement(measurement)
	}
}
