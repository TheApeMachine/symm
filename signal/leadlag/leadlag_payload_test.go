package leadlag

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/transport"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
)

func inefficientLagPayload() []float64 {
	return []float64{0, 100, 1, 1, 0, 1, 8, 0.9, 0, 0, 20}
}

func syncDriftPayload() []float64 {
	return []float64{0, 100, 1, 1, 0, 1, 0, 0.9, 1, 0.9, 20}
}

func decoupledMovePayload() []float64 {
	return []float64{0, 100, 1, 0, 0.5, 0, 0, 0, 1, 0.01, 20}
}

func anchorStallPayload() []float64 {
	return []float64{1, 50000, 1, 0, 0.8, 0, 0, 0, 0, 0, 0}
}

func lagOutcomeFromPayload(payload []float64) algorithm.LagOutcome {
	lag := algorithm.NewLag(datura.Acquire("lag-config", datura.APPJSON))
	processed := datura.Acquire("leadlag", datura.APPJSON)
	processed.WithScope("ETH/EUR")
	processed.WithPayload(equation.MarshalFeatureSchema(equation.LagInputKeys, payload))
	_ = transport.NewFlipFlop(processed, lag)
	processed.Release()

	return lag.Outcome()
}

func TestLagPayloadClassification(testingTB *testing.T) {
	Convey("Given lag payload fixtures", testingTB, func() {
		Convey("It should classify inefficient lag", func() {
			outcome := lagOutcomeFromPayload(inefficientLagPayload())
			So(outcome.Eligible, ShouldBeTrue)
			So(outcome.Category, ShouldEqual, 1)
		})

		Convey("It should classify synchronized drift", func() {
			outcome := lagOutcomeFromPayload(syncDriftPayload())
			So(outcome.Eligible, ShouldBeTrue)
			So(outcome.Category, ShouldEqual, 2)
		})

		Convey("It should classify decoupled move", func() {
			outcome := lagOutcomeFromPayload(decoupledMovePayload())
			So(outcome.Eligible, ShouldBeTrue)
			So(outcome.Category, ShouldEqual, 3)
		})

		Convey("It should classify anchor stall", func() {
			outcome := lagOutcomeFromPayload(anchorStallPayload())
			So(outcome.Eligible, ShouldBeTrue)
			So(outcome.Category, ShouldEqual, 4)
		})
	})
}
