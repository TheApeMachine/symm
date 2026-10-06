package relation

import (
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
fixtureKey renders the coordinate key of one synthetic test coordinate.
*/
func fixtureKey(source string, metric string) string {
	return "TEST/USD|" + source + "|" + metric + "|||||1"
}

/*
appendSeries writes one flattened series at a fixed cadence from the epoch.
*/
func appendSeries(store *ObservationStore, key string, values []float64, step time.Duration) {
	flat := make([]float64, 0, 2*len(values))

	for index, value := range values {
		flat = append(flat, float64(time.Duration(index)*step), value)
	}

	for range store.Next(data.NewValue(map[string][]float64{key: flat})) {
	}
}

/*
residentCopy copies the resident windows out of one read.
*/
func residentCopy(store *ObservationStore) map[string][]float64 {
	copied := make(map[string][]float64)

	for pointer := range store.Next(nil) {
		for key, window := range *(*map[string][]float64)(pointer) {
			copied[key] = append([]float64(nil), window...)
		}
	}

	return copied
}

func TestObservationStoreRetention(t *testing.T) {
	Convey("Given a bounded observation store", t, func() {
		store := NewObservationStore(3)
		key := fixtureKey("s", "m")
		values := []float64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}

		for index, value := range values {
			for range store.Next(data.NewValue(map[string][]float64{
				key: {float64(int64(index) * int64(time.Second)), value},
			})) {
			}
		}

		So(store.Error(), ShouldBeNil)
		window := residentCopy(store)[key]

		Convey("retention is chronological and bounded by the infrastructure capacity", func() {
			So(window, ShouldResemble, []float64{
				float64(7 * time.Second), 7,
				float64(8 * time.Second), 8,
				float64(9 * time.Second), 9,
			})
		})

		Convey("one batch of many observations evicts the same way", func() {
			batched := NewObservationStore(3)
			appendSeries(batched, key, values, time.Second)
			So(residentCopy(batched)[key], ShouldResemble, window)
		})
	})
}

func TestObservationStoreEpochSeparation(t *testing.T) {
	Convey("Given observations in two model epochs", t, func() {
		store := NewObservationStore(64)
		epochOne := "TEST/USD|e|m|||||1"
		epochTwo := "TEST/USD|e|m|||||2"

		for range store.Next(data.NewValue(map[string][]float64{
			epochOne: {1, 1},
			epochTwo: {2, 2},
		})) {
		}

		Convey("incompatible epochs are never mixed", func() {
			resident := residentCopy(store)
			So(resident[epochOne], ShouldResemble, []float64{1, 1})
			So(resident[epochTwo], ShouldResemble, []float64{2, 2})
		})
	})
}

func TestObservationStoreCoordinates(t *testing.T) {
	Convey("Given a store with several coordinates", t, func() {
		store := NewObservationStore(64)

		Convey("an empty store has no coordinates", func() {
			So(residentCopy(store), ShouldBeEmpty)
		})

		for range store.Next(data.NewValue(map[string][]float64{
			fixtureKey("cvd", "signed_net_fraction_zscore"): {1, 1},
			fixtureKey("hawkes", "arrival_rate_zscore"):     {2, 2},
			fixtureKey("cvd", "midpoint_log_return"):        {3, 3},
		})) {
		}

		Convey("every observed coordinate is resident", func() {
			So(residentCopy(store), ShouldHaveLength, 3)
		})

		Convey("an empty batch registers a coordinate without observing it", func() {
			fourth := fixtureKey("cvd", "gross_notional_rate_zscore")

			for range store.Next(data.NewValue(map[string][]float64{fourth: nil})) {
			}

			resident := residentCopy(store)
			So(resident, ShouldHaveLength, 4)
			So(resident[fourth], ShouldBeEmpty)

			Convey("and registration is idempotent", func() {
				for range store.Next(data.NewValue(map[string][]float64{fourth: nil})) {
				}

				So(residentCopy(store), ShouldHaveLength, 4)
			})
		})

		Convey("an unknown coordinate is missing, not zero", func() {
			_, held := residentCopy(store)[fixtureKey("cvd", "missing")]
			So(held, ShouldBeFalse)
		})
	})
}

func TestObservationStoreShape(t *testing.T) {
	Convey("Given invalid store usage", t, func() {
		Convey("a non-positive capacity is a domain failure, not a fallback", func() {
			invalid := NewObservationStore(0)
			So(invalid.Error(), ShouldNotBeNil)

			var yielded int

			for range invalid.Next(data.NewValue(map[string][]float64{"k": {1, 1}})) {
				yielded++
			}

			for range invalid.Next(nil) {
				yielded++
			}

			So(yielded, ShouldEqual, 0)
		})

		Convey("a batch that is not {at, raw} pairs is a shape failure", func() {
			store := NewObservationStore(4)
			var yielded int

			for range store.Next(data.NewValue(map[string][]float64{"k": {1, 1, 2}})) {
				yielded++
			}

			So(yielded, ShouldEqual, 0)
			So(store.Error(), ShouldNotBeNil)
		})
	})
}

var benchmarkStoreSink int

func BenchmarkObservationStoreAppend(b *testing.B) {
	store := NewObservationStore(2048)
	key := fixtureKey("cvd", "signed_net_fraction_zscore")
	batch := map[string][]float64{key: {0, 0}}

	b.ReportAllocs()

	for iteration := 0; b.Loop(); iteration++ {
		batch[key][0] = float64(iteration)
		batch[key][1] = float64(iteration)

		for range store.Next(data.NewValue(batch)) {
		}

		benchmarkStoreSink++
	}
}

func BenchmarkObservationStoreRead(b *testing.B) {
	store := NewObservationStore(2048)

	for index := 0; index < 256; index++ {
		appendSeries(store, fixtureKey(fmt.Sprintf("source%d", index), "metric"), []float64{1, 2, 3}, time.Second)
	}

	b.ReportAllocs()

	for b.Loop() {
		for pointer := range store.Next(nil) {
			benchmarkStoreSink = len(*(*map[string][]float64)(pointer))
		}
	}
}
