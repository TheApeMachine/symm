package trader

import (
	"context"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
)

func TestRecordCalibrationProbe(t *testing.T) {
	previousMinSamples := config.System.MinCalibrationSamples
	config.System.MinCalibrationSamples = 1
	defer func() { config.System.MinCalibrationSamples = previousMinSamples }()

	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	prices := stubPrices{"PUMP/EUR": 100}
	candidatesGroup := pool.CreateBroadcastGroup("candidates", 10*time.Millisecond)
	scorer, err := NewScorer(
		ctx,
		pool,
		nil,
		candidatesGroup,
		prices,
		nil,
		&stubSignal{},
	)

	if err != nil {
		t.Fatalf("new scorer: %v", err)
	}

	start := time.Unix(1_700_000_000, 0)
	scorer.updatePairStates(testMeasurement(0.5), start)
	prices["PUMP/EUR"] = 102
	scorer.settleDuePredictions(start.Add(config.System.ScalpHoldBeforeExit + time.Second))
	gross, ok := scorer.ReturnModel().Predict("hawkes", "momentum", 0.5)

	convey.Convey("Given an uncalibrated signal with a live quote", t, func() {
		convey.Convey("It should learn from an actual forward return before trading", func() {
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(gross, convey.ShouldBeGreaterThan, 0)
		})
	})
}
