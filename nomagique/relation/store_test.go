package relation

import (
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStoreRetention(t *testing.T) {
	Convey("Given a bounded observation store", t, func() {
		store := NewObservationStore(3)
		coordinate := fixtureCoordinate("s", "m")

		for index := 0; index < 10; index++ {
			store.Append(Observation{
				Coordinate: coordinate,
				Raw:        float64(index),
				At:         time.Unix(0, int64(index)*int64(time.Second)),
			})
		}

		Convey("retention is chronological and bounded by the infrastructure capacity", func() {
			history := store.History(coordinate)
			So(len(history), ShouldEqual, 3)
			So(history[0].Raw, ShouldEqual, 7)
			So(history[2].Raw, ShouldEqual, 9)
		})

		Convey("eviction is never value-based", func() {
			So(store.Retention().Capacity, ShouldEqual, 3)
		})

		Convey("snapshots report coordinate and observation counts", func() {
			snapshot := store.Snapshot()
			So(snapshot.Coordinates, ShouldEqual, 1)
			So(snapshot.Observations, ShouldEqual, 3)
			So(snapshot.Appended, ShouldEqual, 10)
		})
	})
}

func TestEpochSeparation(t *testing.T) {
	Convey("Given observations in two model epochs", t, func() {
		store := NewObservationStore(64)
		epochOne := fixtureCoordinate("e", "m")
		epochOne.Epoch = 1
		epochTwo := fixtureCoordinate("e", "m")
		epochTwo.Epoch = 2

		store.Append(Observation{Coordinate: epochOne, Raw: 1, At: time.Unix(1, 0)})
		store.Append(Observation{Coordinate: epochTwo, Raw: 2, At: time.Unix(2, 0)})

		Convey("incompatible epochs are never mixed", func() {
			So(store.Count(epochOne), ShouldEqual, 1)
			So(store.Count(epochTwo), ShouldEqual, 1)

			historyOne := store.History(epochOne)
			historyTwo := store.History(epochTwo)
			So(historyOne[0].Raw, ShouldEqual, 1)
			So(historyTwo[0].Raw, ShouldEqual, 2)
		})
	})
}

func TestCoordinates(t *testing.T) {
	Convey("Given a store with several coordinates", t, func() {
		store := NewObservationStore(64)
		first := fixtureCoordinate("cvd", "signed_net_fraction_zscore")
		second := fixtureCoordinate("hawkes", "arrival_rate_zscore")
		third := fixtureCoordinate("cvd", "midpoint_log_return")

		Convey("an empty store has no coordinates", func() {
			So(store.Coordinates(), ShouldBeEmpty)
		})

		store.Append(Observation{Coordinate: first, Raw: 1, At: time.Unix(1, 0)})
		store.Append(Observation{Coordinate: second, Raw: 2, At: time.Unix(2, 0)})
		store.Append(Observation{Coordinate: third, Raw: 3, At: time.Unix(3, 0)})

		Convey("every observed coordinate is returned in canonical order", func() {
			coordinates := store.Coordinates()
			So(coordinates, ShouldHaveLength, 3)

			for index := 0; index+1 < len(coordinates); index++ {
				So(CompareCoordinate(coordinates[index], coordinates[index+1]) < 0, ShouldBeTrue)
			}
		})

		Convey("ordinary appends neither grow nor reorder the universe", func() {
			before := store.Coordinates()

			for index := 0; index < 100; index++ {
				store.Append(Observation{Coordinate: first, Raw: float64(index), At: time.Unix(int64(index), 0)})
			}

			after := store.Coordinates()
			So(after, ShouldHaveLength, 3)
			So(after[0], ShouldResemble, before[0])
			So(after[1], ShouldResemble, before[1])
			So(after[2], ShouldResemble, before[2])
		})

		Convey("a newly introduced coordinate appears in the index", func() {
			fourth := fixtureCoordinate("cvd", "gross_notional_rate_zscore")
			So(store.Coordinates(), ShouldHaveLength, 3)

			store.Append(Observation{Coordinate: fourth, Raw: 4, At: time.Unix(4, 0)})

			coordinates := store.Coordinates()
			So(coordinates, ShouldHaveLength, 4)
			So(coordinates, ShouldContain, fourth)
		})

		Convey("repeated reads return a stable immutable snapshot", func() {
			firstRead := store.Coordinates()
			secondRead := store.Coordinates()

			So(len(firstRead), ShouldEqual, len(secondRead))

			for index := range firstRead {
				So(firstRead[index], ShouldResemble, secondRead[index])
			}
		})
	})
}

var benchmarkStoreSink int

func BenchmarkObservationStoreCoordinatesCached(b *testing.B) {
	store := NewObservationStore(2048)

	for index := 0; index < 256; index++ {
		store.Append(Observation{
			Coordinate: fixtureCoordinate(fmt.Sprintf("source%d", index), fmt.Sprintf("metric%d", index)),
			Raw:        float64(index),
			At:         time.Unix(0, int64(index)*int64(time.Second)),
		})
	}

	// Warm the immutable index once so the loop measures the cached read.
	store.Coordinates()

	b.ReportAllocs()
	b.ResetTimer()

	for iteration := 0; iteration < b.N; iteration++ {
		benchmarkStoreSink = len(store.Coordinates())
	}
}

func BenchmarkObservationStoreCoordinatesRebuild(b *testing.B) {
	store := NewObservationStore(2048)

	for index := 0; index < 256; index++ {
		store.Append(Observation{
			Coordinate: fixtureCoordinate(fmt.Sprintf("source%d", index), fmt.Sprintf("metric%d", index)),
			Raw:        float64(index),
			At:         time.Unix(0, int64(index)*int64(time.Second)),
		})
	}

	observations := make([]Observation, b.N)

	for index := range observations {
		observations[index] = Observation{
			Coordinate: fixtureCoordinate(fmt.Sprintf("source%d", 256+index), fmt.Sprintf("metric%d", 256+index)),
			Raw:        float64(index),
			At:         time.Unix(0, int64(index)*int64(time.Second)),
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	// Each iteration introduces one brand-new coordinate, forcing exactly one
	// index rebuild: the enumerated, reallocated, re-sorted universe rebuild.
	for iteration := 0; iteration < b.N; iteration++ {
		store.Append(observations[iteration])
		benchmarkStoreSink = len(store.Coordinates())
	}
}
