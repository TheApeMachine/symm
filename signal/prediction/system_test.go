package prediction

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/logic"
)

func TestSystemIngestMeasurement(t *testing.T) {
	Convey("Given prediction subscribed to the measurements bus", t, func() {
		viper.Set("signals.prediction.measurements_capacity", 8)
		viper.Set("story.prediction.horizon", time.Minute)
		viper.Set("story.prediction.alpha", 0.1)
		viper.Set("story.prediction.rls_initial_variance", 1000.0)

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 8, nil)
		publisher := internal.NewBus(ctx, pool, []string{"measurements"}, nil)
		system := NewSystem(ctx, pool)

		So(publisher.Send("measurements", "measurements", logic.Measurement{
			Source:     logic.SourcePumpDump,
			Symbol:     "ETH/EUR",
			Confidence: 0.75,
			ObservedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		}), ShouldBeNil)

		measurementRow, pollErr := system.bus.Poll("measurements")

		So(pollErr, ShouldBeNil)
		So(measurementRow, ShouldNotBeNil)

		So(system.processMessage(measurementRow), ShouldBeNil)

		featureSignal := system.LoadSignal(logic.EntityMeasurement, "ETH/EUR")
		pumpDumpIndex := featureSourceIndex(logic.SourcePumpDump)

		So(pumpDumpIndex, ShouldBeGreaterThanOrEqualTo, 0)
		So(featureSignal.features[pumpDumpIndex], ShouldEqual, 0.75)
	})
}
