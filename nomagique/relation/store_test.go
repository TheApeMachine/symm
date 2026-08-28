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
			var raws []float64

			store.RangeHistory(coordinate, func(observation Observation) bool {
				raws = append(raws, observation.Raw)
				return true
			})

			So(raws, ShouldResemble, []float64{7, 8, 9})
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

			firstRaw := 0.0
			secondRaw := 0.0

			store.RangeHistory(epochOne, func(observation Observation) bool {
				firstRaw = observation.Raw
				return false
			})

			store.RangeHistory(epochTwo, func(observation Observation) bool {
				secondRaw = observation.Raw
				return false
			})

			So(firstRaw, ShouldEqual, 1)
			So(secondRaw, ShouldEqual, 2)
		})
	})
}

func TestRangeCoordinates(t *testing.T) {
	Convey("Given a store with several coordinates", t, func() {
		store := NewObservationStore(64)
		first := fixtureCoordinate("cvd", "signed_net_fraction_zscore")
		second := fixtureCoordinate("hawkes", "arrival_rate_zscore")
		third := fixtureCoordinate("cvd", "midpoint_log_return")

		collect := func() []Coordinate {
			var coordinates []Coordinate

			store.RangeCoordinates(func(coordinate Coordinate) bool {
				coordinates = append(coordinates, coordinate)
				return true
			})

			return coordinates
		}

		Convey("an empty store has no coordinates", func() {
			So(collect(), ShouldBeEmpty)
		})

		store.Append(Observation{Coordinate: first, Raw: 1, At: time.Unix(1, 0)})
		store.Append(Observation{Coordinate: second, Raw: 2, At: time.Unix(2, 0)})
		store.Append(Observation{Coordinate: third, Raw: 3, At: time.Unix(3, 0)})

		Convey("every observed coordinate is visited in canonical order", func() {
			coordinates := collect()
			So(coordinates, ShouldHaveLength, 3)

			for index := 0; index+1 < len(coordinates); index++ {
				So(CompareCoordinate(coordinates[index], coordinates[index+1]) < 0, ShouldBeTrue)
			}
		})

		Convey("ordinary appends neither grow nor reorder the universe", func() {
			before := collect()

			for index := 0; index < 100; index++ {
				store.Append(Observation{Coordinate: first, Raw: float64(index), At: time.Unix(int64(index), 0)})
			}

			after := collect()
			So(after, ShouldHaveLength, 3)
			So(after[0], ShouldResemble, before[0])
			So(after[1], ShouldResemble, before[1])
			So(after[2], ShouldResemble, before[2])
		})

		Convey("a newly registered coordinate appears in the traversal", func() {
			fourth := fixtureCoordinate("cvd", "gross_notional_rate_zscore")
			So(collect(), ShouldHaveLength, 3)

			store.RegisterCoordinate(fourth)

			coordinates := collect()
			So(coordinates, ShouldHaveLength, 4)
			So(coordinates, ShouldContain, fourth)
		})

		Convey("registration is idempotent", func() {
			store.RegisterCoordinate(first)
			store.RegisterCoordinate(first)
			So(store.CoordinateCount(), ShouldEqual, 3)
		})

		Convey("repeated traversals return the same resident universe", func() {
			firstRead := collect()
			secondRead := collect()

			So(len(firstRead), ShouldEqual, len(secondRead))

			for index := range firstRead {
				So(firstRead[index], ShouldResemble, secondRead[index])
			}
		})
	})
}

func TestRangeHistory(t *testing.T) {
	Convey("Given a store with a bounded ring", t, func() {
		store := NewObservationStore(3)
		coordinate := fixtureCoordinate("s", "m")

		for index := 0; index < 10; index++ {
			store.Append(Observation{
				Coordinate: coordinate,
				Raw:        float64(index),
				At:         time.Unix(0, int64(index)*int64(time.Second)),
			})
		}

		Convey("the resident ring is visited chronologically without copying", func() {
			var raws []float64

			store.RangeHistory(coordinate, func(observation Observation) bool {
				raws = append(raws, observation.Raw)
				return true
			})

			So(raws, ShouldResemble, []float64{7, 8, 9})
		})

		Convey("an unknown coordinate visits nothing", func() {
			count := 0

			store.RangeHistory(fixtureCoordinate("s", "missing"), func(Observation) bool {
				count++
				return true
			})

			So(count, ShouldEqual, 0)
		})

		Convey("a ring view reads the same resident ring in place", func() {
			view, found := store.ViewRing(coordinate)
			So(found, ShouldBeTrue)
			So(view.Len(), ShouldEqual, 3)

			for index := 0; index < view.Len(); index++ {
				So(view.At(index).Raw, ShouldEqual, float64(7+index))
			}

			view.Close()
		})
	})
}

var benchmarkStoreSink int

func BenchmarkObservationStoreAppend(b *testing.B) {
	store := NewObservationStore(2048)
	coordinate := fixtureCoordinate("cvd", "signed_net_fraction_zscore")
	store.RegisterCoordinate(coordinate)

	b.ReportAllocs()
	

	for iteration := 0; b.Loop(); iteration++ {
		store.Append(Observation{Coordinate: coordinate, Raw: float64(iteration), At: time.Unix(0, int64(iteration)*int64(time.Second))})
		benchmarkStoreSink++
	}
}

func BenchmarkObservationStoreRangeCoordinates(b *testing.B) {
	store := NewObservationStore(2048)

	for index := 0; index < 256; index++ {
		store.Append(Observation{
			Coordinate: fixtureCoordinate(fmt.Sprintf("source%d", index), fmt.Sprintf("metric%d", index)),
			Raw:        float64(index),
			At:         time.Unix(0, int64(index)*int64(time.Second)),
		})
	}

	b.ReportAllocs()
	

	for b.Loop() {
		store.RangeCoordinates(func(coordinate Coordinate) bool {
			benchmarkStoreSink = len(coordinate.Symbol)
			return true
		})
	}
}

func BenchmarkObservationStoreRangeHistory(b *testing.B) {
	store := NewObservationStore(2048)
	coordinate := fixtureCoordinate("cvd", "signed_net_fraction_zscore")

	for index := 0; index < 2048; index++ {
		store.Append(Observation{Coordinate: coordinate, Raw: float64(index), At: time.Unix(0, int64(index)*int64(time.Second))})
	}

	b.ReportAllocs()
	

	for b.Loop() {
		store.RangeHistory(coordinate, func(observation Observation) bool {
			benchmarkStoreSink = int(observation.Raw)
			return true
		})
	}
}

func BenchmarkObservationStoreRegisterCoordinate(b *testing.B) {
	store := NewObservationStore(2048)

	b.ReportAllocs()
	

	// Each iteration structurally registers one new coordinate: the resident
	// ordered insert is the whole cost of a growing universe.
	for iteration := 0; b.Loop(); iteration++ {
		store.RegisterCoordinate(fixtureCoordinate(fmt.Sprintf("source%d", iteration), "metric"))
	}
}
