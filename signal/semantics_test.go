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
			entry, found := Semantics().Metrics["derivatives/basis"]
			So(found, ShouldBeTrue)
			So(entry.Purpose, ShouldNotBeEmpty)
			So(entry.Forbidden, ShouldNotBeEmpty)
		})

		Convey("An undeclared metric is reported undeclared, never described", func() {
			entry, found := Semantics().Metrics["derivatives/not_a_real_metric"]
			So(found, ShouldBeFalse)
			So(entry.Purpose, ShouldBeEmpty)
			So(entry.Forbidden, ShouldBeEmpty)
		})

		Convey("Decoding is memoised, and answers identically each time", func() {
			So(len(Semantics().Metrics), ShouldEqual, len(declared.Metrics))
		})
	})
}

func TestSignalPurposesTest(t *testing.T) {
	Convey("Given the embedded signal specifications", t, func() {
		declared := SignalPurposes()

		Convey("Every signal family that ships a specification declares its purpose", func() {
			So(len(declared), ShouldBeGreaterThan, 10)

			for source, entry := range declared {
				So(entry.Source, ShouldEqual, source)
				So(entry.Purpose, ShouldNotBeEmpty)
			}
		})

		Convey("A purpose is the specification's own leading statement", func() {
			So(
				declared["hawkes"].Purpose,
				ShouldEqual,
				"The Hawkes signal measures the temporal arrival structure of marked market events.",
			)
		})

		Convey("It stops before the enumerated measurement list", func() {
			for _, entry := range declared {
				So(entry.Purpose, ShouldNotContainSubstring, "It measures:")
				So(entry.Purpose, ShouldNotContainSubstring, "It answers:")
			}
		})

		Convey("It carries no trailing document rule", func() {
			So(declared["liquidity"].Purpose, ShouldNotEndWith, "-")
		})

		Convey("A family with no specification is absent rather than described", func() {
			_, found := declared["not_a_real_signal"]
			So(found, ShouldBeFalse)
		})
	})
}
