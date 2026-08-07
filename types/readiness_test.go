package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestReadinessComplete(t *testing.T) {
	Convey("Given every required stage has stamped the Thesis", t, func() {
		readiness := NewReadiness(nil)

		for _, source := range []SourceType{
			SourceCorrelation,
			SourceCVD,
			SourceDepthFlow,
			SourceExhaustion,
			SourceHawkes,
			SourceLeadLag,
			SourceLiquidity,
			SourcePumpDump,
			SourceSentiment,
			SourceToxicity,
			SourceCategories,
			SourceCognition,
			SourceManifold,
			SourceResonance,
			SourceCausal,
			SourceGraph,
			SourcePlanner,
		} {
			readiness.Stamp(source)
		}

		Convey("Then the complete check should return true", func() {
			So(readiness.Complete(), ShouldBeTrue)
		})
	})
}
