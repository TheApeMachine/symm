package broker

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestStressCacheIngestMeasurement(t *testing.T) {
	Convey("Given a stress cache", t, func() {
		cache := NewStressCache(context.Background(), nil)

		Convey("It should record toxicity readings", func() {
			cache.ingestMeasurement(perspectives.Measurement{
				Symbol:   "BTC/EUR",
				Source:   perspectives.SourceToxicity,
				Category: perspectives.CategoryToxicBluff,
				SNR:      1.2,
			})

			stress := cache.Snapshot("BTC/EUR")

			So(stress.ToxicityCategory, ShouldEqual, perspectives.CategoryToxicBluff)
			So(stress.ToxicitySNR, ShouldAlmostEqual, 1.2, 1e-9)
		})

		Convey("It should record fluid readings", func() {
			cache.ingestMeasurement(perspectives.Measurement{
				Symbol:   "BTC/EUR",
				Source:   perspectives.SourceFluid,
				Category: perspectives.CategoryTurbulent,
				SNR:      0.8,
			})

			stress := cache.Snapshot("BTC/EUR")

			So(stress.FluidCategory, ShouldEqual, perspectives.CategoryTurbulent)
			So(stress.FluidSNR, ShouldAlmostEqual, 0.8, 1e-9)
		})

		Convey("It should record sentiment readings", func() {
			cache.ingestMeasurement(perspectives.Measurement{
				Symbol:   "BTC/EUR",
				Source:   perspectives.SourceSentiment,
				Category: perspectives.CategorySystemicSlump,
				SNR:      2.1,
			})

			stress := cache.Snapshot("BTC/EUR")

			So(stress.SentimentCategory, ShouldEqual, perspectives.CategorySystemicSlump)
			So(stress.SentimentSNR, ShouldAlmostEqual, 2.1, 1e-9)
		})

		Convey("It should ignore unknown sources", func() {
			cache.ingestMeasurement(perspectives.Measurement{
				Symbol: "ETH/EUR",
				Source: perspectives.SourceExhaustion,
				SNR:    9,
			})

			So(cache.Snapshot("ETH/EUR"), ShouldResemble, SymbolStress{})
		})

		Convey("It should ignore empty symbols", func() {
			cache.ingestMeasurement(perspectives.Measurement{
				Source: perspectives.SourceHawkes,
				SNR:    1,
			})

			So(cache.Snapshot(""), ShouldResemble, SymbolStress{})
		})
	})
}

func TestEnsureStressCache(t *testing.T) {
	Convey("Given a pool", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		pool := qpool.NewQ(ctx, 1, 4, nil)

		defer func() {
			cancel()
			ResetStressCacheForTest()
			pool.Close()
		}()

		Convey("It should return a shared cache instance", func() {
			first := EnsureStressCache(ctx, pool)
			second := EnsureStressCache(ctx, pool)

			So(first, ShouldEqual, second)
		})
	})
}

func TestEnsureStressCacheRecyclesOnCancel(t *testing.T) {
	Convey("Given a canceled stress-cache context", t, func() {
		firstCtx, firstCancel := context.WithCancel(context.Background())
		firstPool := qpool.NewQ(firstCtx, 1, 4, nil)

		first := EnsureStressCache(firstCtx, firstPool)
		firstCancel()
		ResetStressCacheForTest()
		firstPool.Close()

		secondCtx, secondCancel := context.WithCancel(context.Background())
		defer secondCancel()

		secondPool := qpool.NewQ(secondCtx, 1, 4, nil)
		defer func() {
			ResetStressCacheForTest()
			secondPool.Close()
		}()

		second := EnsureStressCache(secondCtx, secondPool)

		Convey("It should construct a new cache instance", func() {
			So(fmt.Sprintf("%p", second), ShouldNotEqual, fmt.Sprintf("%p", first))
		})
	})
}

func TestStressCacheBroadcast(t *testing.T) {
	Convey("Given a stress cache subscribed to measurements", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		defer pool.Close()

		cache := NewStressCache(ctx, nil)
		cache.Start(pool)

		group := pool.CreateBroadcastGroup("measurements", 0)
		measurement := perspectives.Measurement{
			Symbol:   "BTC/EUR",
			Source:   perspectives.SourceToxicity,
			Category: perspectives.CategoryToxicBluff,
			SNR:      1.5,
		}

		deadline := time.Now().Add(2 * time.Second)
		var stress SymbolStress

		for time.Now().Before(deadline) {
			group.Send(&qpool.QValue[any]{Value: measurement})
			stress = cache.Snapshot("BTC/EUR")

			if stress.ToxicityCategory == perspectives.CategoryToxicBluff {
				break
			}

			time.Sleep(5 * time.Millisecond)
		}

		Convey("It should ingest broadcast measurements", func() {
			So(stress.ToxicityCategory, ShouldEqual, perspectives.CategoryToxicBluff)
			So(stress.ToxicitySNR, ShouldAlmostEqual, 1.5, 1e-9)
		})
	})
}

func BenchmarkStressCacheSnapshot(b *testing.B) {
	cache := NewStressCache(context.Background(), nil)
	cache.InstallStressForTest("BTC/EUR", SymbolStress{
		ToxicityCategory: perspectives.CategoryToxicBluff,
		ToxicitySNR:      1,
	})

	for b.Loop() {
		_ = cache.Snapshot("BTC/EUR")
	}
}

func BenchmarkStressCacheIngestMeasurement(b *testing.B) {
	cache := NewStressCache(context.Background(), nil)
	measurement := perspectives.Measurement{
		Symbol:   "BTC/EUR",
		Source:   perspectives.SourceHawkes,
		Category: perspectives.CategorySaturation,
		SNR:      2.5,
	}

	for b.Loop() {
		cache.ingestMeasurement(measurement)
	}
}
