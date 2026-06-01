package optimizer

import (
	"context"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestTunerTick(t *testing.T) {
	convey.Convey("Given a tuner subscribed to measurements", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		tuner := NewTuner(ctx, pool)
		measurements := pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)

		go func() {
			_ = tuner.Tick()
		}()

		measurements.Send(&qpool.QValue[any]{
			Value: perspectives.Measurement{Symbol: "BTC/EUR"},
		})
		cancel()

		time.Sleep(20 * time.Millisecond)

		convey.Convey("It should count measurements until canceled", func() {
			convey.So(tuner.MeasurementCount(), convey.ShouldEqual, 1)
		})
	})
}

func BenchmarkTunerMeasurementCount(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 1, 4, nil)
	tuner := NewTuner(ctx, pool)

	b.ReportAllocs()

	for b.Loop() {
		tuner.measurementCount.Add(1)
		_ = tuner.MeasurementCount()
	}
}
