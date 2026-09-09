package hindsight

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/hindsight/tables/tablestest"
)

func TestReadObservations(t *testing.T) {
	Convey("An archive cursor advances only over completely decoded capture suffixes", t, func() {
		catalog := tablestest.New(t)
		writer := tables.NewWriter(catalog)
		writer.AddCapture(tables.CaptureRow{Run: "run", Sequence: 2, Kind: "heartbeat", Payload: []byte(`{}`)})
		writer.AddCapture(tables.CaptureRow{Run: "run", Sequence: 1, Kind: "ticker", Payload: []byte(`{"data":[{"symbol":"BTC/USD","bid":100},{"symbol":"ETH/USD","bid":10}]}`)})
		writer.AddCapture(tables.CaptureRow{Run: "other", Sequence: 3, Kind: "ticker", Payload: []byte(`invalid`)})
		So(writer.Commit(t.Context()), ShouldBeNil)
		observations, through, err := ReadObservations(t.Context(), catalog, "run", 0)
		So(err, ShouldBeNil)
		So(through, ShouldEqual, 2)
		So(len(observations), ShouldEqual, 2)
		So(observations[0].Symbol, ShouldEqual, "BTC/USD")
		So(observations[1].Ordinal, ShouldEqual, 1)

		Convey("Caught-up reads return no observations and preserve the cursor", func() {
			suffix, next, err := ReadObservations(t.Context(), catalog, "run", through)
			So(err, ShouldBeNil)
			So(len(suffix), ShouldEqual, 0)
			So(next, ShouldEqual, through)
		})

		Convey("A new commit returns only its new observations", func() {
			writer.AddCapture(tables.CaptureRow{Run: "run", Sequence: 3, Kind: "ticker", Payload: []byte(`{"data":[{"symbol":"BTC/USD","bid":102}]}`)})
			So(writer.Commit(t.Context()), ShouldBeNil)
			suffix, next, err := ReadObservations(t.Context(), catalog, "run", through)
			So(err, ShouldBeNil)
			So(len(suffix), ShouldEqual, 1)
			So(suffix[0].Bid, ShouldEqual, 102)
			So(next, ShouldEqual, 3)
		})

		Convey("A malformed suffix cannot advance or partially publish the cursor", func() {
			writer.AddCapture(tables.CaptureRow{Run: "run", Sequence: 3, Kind: "ticker", Payload: []byte(`{"data":[{"symbol":"BTC/USD","bid":102}]}`)})
			writer.AddCapture(tables.CaptureRow{Run: "run", Sequence: 4, Kind: "ticker", Payload: []byte(`invalid`)})
			So(writer.Commit(t.Context()), ShouldBeNil)
			suffix, next, err := ReadObservations(t.Context(), catalog, "run", through)
			So(err, ShouldNotBeNil)
			So(suffix, ShouldBeNil)
			So(next, ShouldEqual, through)
		})
	})
}

func BenchmarkReadObservations(b *testing.B) {
	catalog := tablestest.New(b)
	writer := tables.NewWriter(catalog)
	// A committed 4096-frame prefix and a separate 16-frame suffix model
	// archive growth. These are storage fixture sizes, not market windows.
	for sequence := int64(1); sequence <= 4112; sequence++ {
		writer.AddCapture(tables.CaptureRow{
			Run: "bench", Sequence: sequence, Kind: "ticker",
			Payload: []byte(`{"data":[{"symbol":"BTC/USD","bid":100,"ask":101,"last":100.5}]}`),
		})
		if sequence == 4096 || sequence == 4112 {
			if err := writer.Commit(b.Context()); err != nil {
				b.Fatal(err)
			}
		}
	}
	for _, fixture := range []struct {
		name  string
		after int64
		count int
	}{{"whole_run", 0, 4112}, {"new_suffix", 4096, 16}, {"caught_up", 4112, 0}} {
		b.Run(fixture.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				observations, through, err := ReadObservations(b.Context(), catalog, "bench", fixture.after)
				if err != nil {
					b.Fatal(err)
				}
				if len(observations) != fixture.count || through != 4112 {
					b.Fatal("incorrect archive suffix", len(observations), through)
				}
			}
		})
	}
}
