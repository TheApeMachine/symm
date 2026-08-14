package hawkes

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/hawkes"
)

func TestRevision_Observe(t *testing.T) {
	Convey("Given an observed two-sided window", t, func() {
		start := time.Unix(1000, 0)
		revision := &revision{}
		initial := hawkes.NewArrivalStream(
			[]time.Time{start, start.Add(2 * time.Second)},
			[]time.Time{start.Add(time.Second)},
		)
		So(revision.Observe(initial), ShouldEqual, 3)

		Convey("It should ignore an unchanged replay", func() {
			So(revision.Observe(initial), ShouldEqual, 0)
		})

		Convey("It should count one event entering a rolling replacement", func() {
			replaced := hawkes.NewArrivalStream(
				[]time.Time{start.Add(2 * time.Second), start.Add(4 * time.Second)},
				[]time.Time{start.Add(time.Second)},
			)

			So(revision.Observe(replaced), ShouldEqual, 1)
		})
	})
}
