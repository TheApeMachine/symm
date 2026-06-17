package signal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
)

func TestInsertMeasurement(testingTB *testing.T) {
	Convey("Given a publishable measurement artifact", testingTB, func() {
		tree := dmt.NewTree("")
		artifact := datura.Acquire("hawkes", datura.Artifact_Type_json)
		artifact.WithRole("measurement")
		artifact.WithScope("BTC/EUR")
		artifact.WithAttribute("classifier.category", 2)
		artifact.WithAttribute("classifier.confidence", 0.75)

		InsertMeasurement(tree, artifact)

		Convey("It should index the row under measurement scope origin", func() {
			var found bool

			for inbound := range tree.Seek([]byte("measurement/BTC/EUR/hawkes")) {
				found = true
				inbound.Release()
			}

			So(found, ShouldBeTrue)
		})

		artifact.Release()
	})

	Convey("Given an unpublished measurement artifact", testingTB, func() {
		tree := dmt.NewTree("")
		artifact := datura.Acquire("hawkes", datura.Artifact_Type_json)
		artifact.WithScope("ETH/EUR")

		InsertMeasurement(tree, artifact)

		Convey("It should not index the row", func() {
			var found bool

			for inbound := range tree.Seek([]byte("measurement/ETH/EUR/hawkes")) {
				found = true
				inbound.Release()
			}

			So(found, ShouldBeFalse)
		})

		artifact.Release()
	})
}

func BenchmarkInsertMeasurement(benchmark *testing.B) {
	tree := dmt.NewTree("")
	artifact := datura.Acquire("hawkes", datura.Artifact_Type_json)
	artifact.WithScope("BTC/EUR")
	artifact.WithAttribute("classifier.category", 2)
	artifact.WithAttribute("classifier.confidence", 0.75)

	benchmark.ReportAllocs()
	benchmark.ResetTimer()

	for benchmark.Loop() {
		InsertMeasurement(tree, artifact)
	}

	artifact.Release()
}
