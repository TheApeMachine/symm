package store_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/store"
)

func TestWriteNext(t *testing.T) {
	Convey("Write binds an arriving value to a key as a latest-store command", t, func() {
		latest := store.NewLatest[string, float64]()
		pipeline := nomagique.NewNumber(
			store.NewWrite[string, float64]("signed_correlation"),
			latest,
		)

		reading := sequence.Read[store.LatestReading[string, float64]](
			pipeline.Next(sequence.NewValue(0.5)),
		)
		So(reading.Key, ShouldEqual, "signed_correlation")
		So(reading.Current, ShouldEqual, 0.5)
		So(reading.HasPrior, ShouldBeFalse)

		command := store.LatestCommand[string, float64]{Read: true}
		snapshot := sequence.Read[map[string]float64](latest.Next(sequence.NewValue(command)))
		So(snapshot["signed_correlation"], ShouldEqual, 0.5)
	})
}
