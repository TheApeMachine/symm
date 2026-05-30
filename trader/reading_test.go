package trader

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestTimedMeasurementStale(t *testing.T) {
	convey.Convey("Given a timed measurement with a learned freshness window", t, func() {
		slot := timedMeasurement{
			Measurement: perspectives.Measurement{Category: perspectives.CategoryAggressiveDrive},
			At:          time.Unix(100, 0),
			TTL:         time.Second,
		}

		convey.Convey("It should stay live inside the window", func() {
			convey.So(slot.Stale(time.Unix(100, 500_000_000)), convey.ShouldBeFalse)
		})

		convey.Convey("It should expire after the source misses its own cadence", func() {
			convey.So(slot.Stale(time.Unix(102, 0)), convey.ShouldBeTrue)
		})
	})
}

func TestSnapshotTimedMeasurements(t *testing.T) {
	convey.Convey("Given live and stale source slots", t, func() {
		now := time.Unix(100, 0)
		set := map[perspectives.SourceType]timedMeasurement{
			perspectives.SourceCVD: {
				Measurement: perspectives.Measurement{Category: perspectives.CategoryAggressiveDrive},
				At:          now,
				TTL:         time.Second,
			},
			perspectives.SourcePumpDump: {
				Measurement: perspectives.Measurement{Category: perspectives.CategoryVerticalIgnition},
				At:          now.Add(-2 * time.Second),
				TTL:         time.Second,
			},
		}

		convey.Convey("It should return only non-stale measurements", func() {
			measurements := snapshotTimedMeasurements(set, now)

			convey.So(measurements, convey.ShouldHaveLength, 1)
			convey.So(measurements[0].Category, convey.ShouldEqual, perspectives.CategoryAggressiveDrive)
		})
	})
}

func BenchmarkSnapshotTimedMeasurements(b *testing.B) {
	now := time.Unix(100, 0)
	set := map[perspectives.SourceType]timedMeasurement{
		perspectives.SourceCVD: {
			Measurement: perspectives.Measurement{Category: perspectives.CategoryAggressiveDrive},
			At:          now,
			TTL:         time.Second,
		},
		perspectives.SourcePumpDump: {
			Measurement: perspectives.Measurement{Category: perspectives.CategoryVerticalIgnition},
			At:          now,
			TTL:         time.Second,
		},
	}

	for b.Loop() {
		_ = snapshotTimedMeasurements(set, now)
	}
}
