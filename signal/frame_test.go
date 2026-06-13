package signal

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/logic"
	symmmarket "github.com/theapemachine/symm/market"
)

func TestFinalizeMeasurementFrame(t *testing.T) {
	testconfig.Load(t)

	Convey("Given a shared touch registry", t, func() {
		ctx, cancel, pool := newSystemTestPool(t)
		defer cancel()

		registry := symmmarket.NewTestTouchRegistry(t, ctx, pool)
		now := time.Now().UTC()

		registry.SeedTouch(symmmarket.TouchSnapshot{
			Symbol:     "BTC/USD",
			Bid:        99,
			Ask:        101,
			Last:       100,
			ObservedAt: now,
		})

		measurement := logic.Measurement{
			Source:     logic.SourceCVD,
			Symbol:     "BTC/USD",
			Price:      50,
			Strength:   1,
			Volume:     1,
			Spread:     0.5,
			Elapsed:    1,
			Category:   logic.CategoryAggressiveDrive,
			Confidence: 0.8,
			Surprise:   0.5,
			ObservedAt: now,
		}

		Convey("It should stamp touch prices and mark the frame executable", func() {
			finalized := finalizeMeasurementFrame(measurement, now)

			So(finalized.DecisionGrade, ShouldEqual, logic.DecisionGradeExecutable)
			So(finalized.Price, ShouldEqual, 100)
			So(finalized.Spread, ShouldEqual, 2)
			So(finalized.ObservedAt, ShouldResemble, now)
		})
	})
}

func BenchmarkFinalizeMeasurementFrame(b *testing.B) {
	configTest := &testing.T{}
	testconfig.Load(configTest)

	ctx, cancel, pool := newSystemTestPool(configTest)
	defer cancel()

	registry := symmmarket.NewTestTouchRegistry(configTest, ctx, pool)
	now := time.Now().UTC()

	registry.SeedTouch(symmmarket.TouchSnapshot{
		Symbol:     "BTC/USD",
		Bid:        99,
		Ask:        101,
		Last:       100,
		ObservedAt: now,
	})

	measurement := logic.Measurement{
		Source:     logic.SourceCVD,
		Symbol:     "BTC/USD",
		Price:      50,
		Strength:   1,
		Volume:     1,
		Spread:     0.5,
		Elapsed:    1,
		Category:   logic.CategoryAggressiveDrive,
		Confidence: 0.8,
		Surprise:   0.5,
		ObservedAt: now,
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = finalizeMeasurementFrame(measurement, now)
	}
}

func TestEntityFusionOrder(t *testing.T) {
	Convey("Given accepted entities", t, func() {
		Convey("Flow sources prefer the trade entity", func() {
			order := entityFusionOrder(logic.SourceCVD, []logic.EntityType{
				logic.EntityTrade,
			})

			So(order, ShouldResemble, []logic.EntityType{logic.EntityTrade})
		})

		Convey("Composite sources prefer book before trade", func() {
			order := entityFusionOrder(logic.SourcePumpDump, []logic.EntityType{
				logic.EntityTrade,
				logic.EntityBook,
			})

			So(order[0], ShouldEqual, logic.EntityBook)
			So(order[1], ShouldEqual, logic.EntityTrade)
		})
	})
}
