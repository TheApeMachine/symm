package relation

import (
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
