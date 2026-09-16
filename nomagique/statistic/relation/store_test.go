package relation

import (
	"fmt"
	"testing"
	"time"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
)

/*
driveStore executes one store command and returns the single result.
*/
func driveStore(store core.Primitive, command *StoreCommand) *StoreResult {
	var result *StoreResult

	for out := range store.Next(singlePointer(unsafe.Pointer(command))) {
		result = (*StoreResult)(out)
	}

	return result
}

func TestObservationStoreRetention(t *testing.T) {
	Convey("Given a bounded observation store", t, func() {
		store := NewObservationStore(3)
		coordinate := fixtureCoordinate("s", "m")

		for index := 0; index < 10; index++ {
			driveStore(store, &StoreCommand{Append: &Observation{
				Coordinate: coordinate,
				Raw:        float64(index),
				At:         time.Unix(0, int64(index)*int64(time.Second)),
			}})
		}

		Convey("retention is chronological and bounded by the infrastructure capacity", func() {
			result := driveStore(store, &StoreCommand{History: &HistoryRequest{Coordinate: coordinate}})
			raws := make([]float64, 0, len(result.Observations))

			for _, observation := range result.Observations {
				raws = append(raws, observation.Raw)
			}

			So(raws, ShouldResemble, []float64{7, 8, 9})
		})

		Convey("eviction is never value-based", func() {
			result := driveStore(store, &StoreCommand{Snapshot: &SnapshotRequest{}})
			So(result.Snapshot.Capacity, ShouldEqual, 3)
		})

		Convey("snapshots report coordinate and observation counts", func() {
			result := driveStore(store, &StoreCommand{Snapshot: &SnapshotRequest{}})
			So(result.Snapshot.Coordinates, ShouldEqual, 1)
			So(result.Snapshot.Observations, ShouldEqual, 3)
			So(result.Snapshot.Appended, ShouldEqual, 10)
		})
	})
}

func TestObservationStoreEpochSeparation(t *testing.T) {
	Convey("Given observations in two model epochs", t, func() {
		store := NewObservationStore(64)
		epochOne := fixtureCoordinate("e", "m")
		epochOne.Epoch = 1
		epochTwo := fixtureCoordinate("e", "m")
		epochTwo.Epoch = 2

		driveStore(store, &StoreCommand{Append: &Observation{Coordinate: epochOne, Raw: 1, At: time.Unix(1, 0)}})
		driveStore(store, &StoreCommand{Append: &Observation{Coordinate: epochTwo, Raw: 2, At: time.Unix(2, 0)}})

		Convey("incompatible epochs are never mixed", func() {
			first := driveStore(store, &StoreCommand{History: &HistoryRequest{Coordinate: epochOne}})
			So(first.Observations[0].Raw, ShouldEqual, 1)

			second := driveStore(store, &StoreCommand{History: &HistoryRequest{Coordinate: epochTwo}})
			So(second.Observations[0].Raw, ShouldEqual, 2)
		})
	})
}

func TestObservationStoreCoordinates(t *testing.T) {
	Convey("Given a store with several coordinates", t, func() {
		store := NewObservationStore(64)
		first := fixtureCoordinate("cvd", "signed_net_fraction_zscore")
		second := fixtureCoordinate("hawkes", "arrival_rate_zscore")
		third := fixtureCoordinate("cvd", "midpoint_log_return")

		collect := func() []Coordinate {
			result := driveStore(store, &StoreCommand{Coordinates: &CoordinateScope{}})
			return result.Coordinates
		}

		Convey("an empty store has no coordinates", func() {
			So(collect(), ShouldBeEmpty)
		})

		driveStore(store, &StoreCommand{Append: &Observation{Coordinate: first, Raw: 1, At: time.Unix(1, 0)}})
		driveStore(store, &StoreCommand{Append: &Observation{Coordinate: second, Raw: 2, At: time.Unix(2, 0)}})
		driveStore(store, &StoreCommand{Append: &Observation{Coordinate: third, Raw: 3, At: time.Unix(3, 0)}})

		Convey("every observed coordinate is visited in canonical order", func() {
			coordinates := collect()
			So(coordinates, ShouldHaveLength, 3)

			for index := 0; index+1 < len(coordinates); index++ {
				So(compareCoordinate(coordinates[index], coordinates[index+1]) < 0, ShouldBeTrue)
			}
		})

		Convey("the symbol scope returns only that symbol's coordinates", func() {
			result := driveStore(store, &StoreCommand{Coordinates: &CoordinateScope{Symbol: "TEST/USD"}})
			So(result.Coordinates, ShouldHaveLength, 3)
		})

		Convey("ordinary appends neither grow nor reorder the universe", func() {
			before := collect()

			for index := 0; index < 100; index++ {
				driveStore(store, &StoreCommand{Append: &Observation{
					Coordinate: first,
					Raw:        float64(index),
					At:         time.Unix(int64(index), 0),
				}})
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

			driveStore(store, &StoreCommand{Register: &fourth})

			coordinates := collect()
			So(coordinates, ShouldHaveLength, 4)
			So(coordinates, ShouldContain, fourth)
		})

		Convey("registration is idempotent", func() {
			driveStore(store, &StoreCommand{Register: &first})
			driveStore(store, &StoreCommand{Register: &first})

			result := driveStore(store, &StoreCommand{Coordinates: &CoordinateScope{}})
			So(result.Coordinates, ShouldHaveLength, 3)
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

func TestObservationStoreRing(t *testing.T) {
	Convey("Given a store with a bounded ring", t, func() {
		store := NewObservationStore(3)
		coordinate := fixtureCoordinate("s", "m")

		for index := 0; index < 10; index++ {
			driveStore(store, &StoreCommand{Append: &Observation{
				Coordinate: coordinate,
				Raw:        float64(index),
				At:         time.Unix(0, int64(index)*int64(time.Second)),
			}})
		}

		Convey("the resident ring is visited chronologically without copying", func() {
			result := driveStore(store, &StoreCommand{History: &HistoryRequest{Coordinate: coordinate}})
			raws := make([]float64, 0, len(result.Observations))

			for _, observation := range result.Observations {
				raws = append(raws, observation.Raw)
			}

			So(raws, ShouldResemble, []float64{7, 8, 9})
		})

		Convey("an unknown coordinate visits nothing and reports missing", func() {
			result := driveStore(store, &StoreCommand{History: &HistoryRequest{
				Coordinate: fixtureCoordinate("s", "missing"),
			}})
			So(result.Found, ShouldBeFalse)
			So(result.Observations, ShouldBeEmpty)
		})

		Convey("a ring view reads the same resident ring in place", func() {
			result := driveStore(store, &StoreCommand{Ring: &RingRequest{Coordinate: coordinate}})
			So(result.Found, ShouldBeTrue)
			So(result.Ring.Len(), ShouldEqual, 3)

			for index := 0; index < result.Ring.Len(); index++ {
				So(result.Ring.At(index).Raw, ShouldEqual, float64(7+index))
			}

			result.Ring.Close()
		})

		Convey("TimeAt reads only the timestamp of the resident ring", func() {
			result := driveStore(store, &StoreCommand{Ring: &RingRequest{Coordinate: coordinate}})
			So(result.Found, ShouldBeTrue)
			defer result.Ring.Close()

			for index := 0; index < result.Ring.Len(); index++ {
				So(result.Ring.TimeAt(index), ShouldEqual, result.Ring.At(index).At)
			}
		})

		Convey("latest and count read the ring head", func() {
			latest := driveStore(store, &StoreCommand{Latest: &LatestRequest{Coordinate: coordinate}})

			So(latest.Found, ShouldBeTrue)
			So(latest.Observation.Raw, ShouldEqual, 9)

			count := driveStore(store, &StoreCommand{Count: &CountRequest{Coordinate: coordinate}})
			So(count.Count, ShouldEqual, 3)
		})

		Convey("the time range spans the retained data", func() {
			result := driveStore(store, &StoreCommand{TimeRange: &TimeRangeRequest{}})
			So(result.TimeFound, ShouldBeTrue)
			So(result.From, ShouldEqual, time.Unix(0, 7*int64(time.Second)))
			So(result.To, ShouldEqual, time.Unix(0, 9*int64(time.Second)))
		})

		Convey("the version counts every append", func() {
			result := driveStore(store, &StoreCommand{Version: &VersionRequest{}})
			So(result.Version, ShouldEqual, 10)
		})

		Convey("a non-positive capacity is a domain failure, not a fallback", func() {
			invalid := NewObservationStore(0)
			So(invalid.Error(), ShouldNotBeNil)

			var yielded int

			for range invalid.Next(singlePointer(unsafe.Pointer(&StoreCommand{}))) {
				yielded++
			}

			So(yielded, ShouldEqual, 0)
		})

		Convey("a command with two intents is a shape failure", func() {
			So(store.Error(), ShouldBeNil)

			var yielded int

			for range store.Next(singlePointer(unsafe.Pointer(&StoreCommand{
				Snapshot: &SnapshotRequest{},
				Version:  &VersionRequest{},
			}))) {
				yielded++
			}

			So(yielded, ShouldEqual, 0)
			So(store.Error(), ShouldNotBeNil)
		})
	})
}

func TestObservationStoreAppendAll(t *testing.T) {
	Convey("Given a batch of observations", t, func() {
		store := NewObservationStore(64)
		coordinate := fixtureCoordinate("s", "m")
		batch := []Observation{
			{Coordinate: coordinate, Raw: 1, At: time.Unix(1, 0)},
			{Coordinate: coordinate, Raw: 2, At: time.Unix(2, 0)},
		}

		driveStore(store, &StoreCommand{AppendAll: &batch})

		Convey("every observation is retained", func() {
			result := driveStore(store, &StoreCommand{History: &HistoryRequest{Coordinate: coordinate}})
			So(result.Observations, ShouldHaveLength, 2)
			So(result.Observations[0].Raw, ShouldEqual, 1)
			So(result.Observations[1].Raw, ShouldEqual, 2)
		})
	})
}

var benchmarkStoreSink int

func BenchmarkObservationStoreAppend(b *testing.B) {
	store := NewObservationStore(2048)
	coordinate := fixtureCoordinate("cvd", "signed_net_fraction_zscore")
	driveStore(store, &StoreCommand{Register: &coordinate})

	b.ReportAllocs()

	for iteration := 0; b.Loop(); iteration++ {
		driveStore(store, &StoreCommand{Append: &Observation{
			Coordinate: coordinate,
			Raw:        float64(iteration),
			At:         time.Unix(0, int64(iteration)*int64(time.Second)),
		}})
		benchmarkStoreSink++
	}
}

func BenchmarkObservationStoreCoordinates(b *testing.B) {
	store := NewObservationStore(2048)

	for index := 0; index < 256; index++ {
		driveStore(store, &StoreCommand{Append: &Observation{
			Coordinate: fixtureCoordinate(fmt.Sprintf("source%d", index), fmt.Sprintf("metric%d", index)),
			Raw:        float64(index),
			At:         time.Unix(0, int64(index)*int64(time.Second)),
		}})
	}

	b.ReportAllocs()

	for b.Loop() {
		result := driveStore(store, &StoreCommand{Coordinates: &CoordinateScope{}})
		benchmarkStoreSink = len(result.Coordinates)
	}
}

func BenchmarkObservationStoreHistory(b *testing.B) {
	store := NewObservationStore(2048)
	coordinate := fixtureCoordinate("cvd", "signed_net_fraction_zscore")

	for index := 0; index < 2048; index++ {
		driveStore(store, &StoreCommand{Append: &Observation{
			Coordinate: coordinate,
			Raw:        float64(index),
			At:         time.Unix(0, int64(index)*int64(time.Second)),
		}})
	}

	b.ReportAllocs()

	for b.Loop() {
		result := driveStore(store, &StoreCommand{History: &HistoryRequest{Coordinate: coordinate}})
		benchmarkStoreSink = len(result.Observations)
	}
}

func BenchmarkObservationStoreRegister(b *testing.B) {
	store := NewObservationStore(2048)

	b.ReportAllocs()

	// Each iteration structurally registers one new coordinate: the resident
	// ordered insert is the whole cost of a growing universe.
	for iteration := 0; b.Loop(); iteration++ {
		coordinate := fixtureCoordinate(fmt.Sprintf("source%d", iteration), "metric")
		driveStore(store, &StoreCommand{Register: &coordinate})
	}
}

func BenchmarkRingViewTimeAt(b *testing.B) {
	store := NewObservationStore(2048)
	coordinate := fixtureCoordinate("cvd", "signed_net_fraction_zscore")

	for index := 0; index < 2048; index++ {
		driveStore(store, &StoreCommand{Append: &Observation{
			Coordinate: coordinate,
			Raw:        float64(index),
			At:         time.Unix(0, int64(index)*int64(time.Second)),
		}})
	}

	result := driveStore(store, &StoreCommand{Ring: &RingRequest{Coordinate: coordinate}})
	defer result.Ring.Close()

	b.ReportAllocs()

	for b.Loop() {
		benchmarkStoreSink = int(result.Ring.TimeAt(1024).UnixNano())
	}
}

func BenchmarkRingViewAt(b *testing.B) {
	store := NewObservationStore(2048)
	coordinate := fixtureCoordinate("cvd", "signed_net_fraction_zscore")

	for index := 0; index < 2048; index++ {
		driveStore(store, &StoreCommand{Append: &Observation{
			Coordinate: coordinate,
			Raw:        float64(index),
			At:         time.Unix(0, int64(index)*int64(time.Second)),
		}})
	}

	result := driveStore(store, &StoreCommand{Ring: &RingRequest{Coordinate: coordinate}})
	defer result.Ring.Close()

	b.ReportAllocs()

	for b.Loop() {
		benchmarkStoreSink = int(result.Ring.At(1024).Raw)
	}
}
