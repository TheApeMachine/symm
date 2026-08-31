package signal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSemanticsTest(t *testing.T) {
	Convey("Given the embedded metric semantic map", t, func() {
		declared := Semantics()

		Convey("It decodes every declared metric identity", func() {
			So(declared.BaselineCommit, ShouldNotBeEmpty)
			So(len(declared.Metrics), ShouldBeGreaterThan, 400)
		})

		Convey("Every entry keeps the identity it is keyed by", func() {
			for identity, entry := range declared.Metrics {
				So(entry.Identity, ShouldEqual, identity)
				So(entry.Source, ShouldNotBeEmpty)
				So(entry.Metric, ShouldNotBeEmpty)
			}
		})

		Convey("A declared metric answers with its own statements", func() {
			entry, found := Lookup("derivatives", "basis")
			So(found, ShouldBeTrue)
			So(entry.Purpose, ShouldNotBeEmpty)
			So(entry.Forbidden, ShouldNotBeEmpty)
		})

		Convey("An undeclared metric is reported undeclared, never described", func() {
			entry, found := Lookup("derivatives", "not_a_real_metric")
			So(found, ShouldBeFalse)
			So(entry.Purpose, ShouldBeEmpty)
			So(entry.Forbidden, ShouldBeEmpty)
		})

		Convey("Decoding is memoised, and answers identically each time", func() {
			So(len(Semantics().Metrics), ShouldEqual, len(declared.Metrics))
		})
	})
}
