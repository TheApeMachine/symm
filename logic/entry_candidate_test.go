package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCorrelatedConfidence(t *testing.T) {
	Convey("Given correlated confidences", t, func() {
		merged := correlatedConfidence([]float64{0.8, 0.75}, 0.35)

		Convey("It should combine conservatively", func() {
			So(merged, ShouldBeLessThan, 0.8)
			So(merged, ShouldBeGreaterThan, 0.4)
		})
	})
}

func TestNormalizedStrengthUsesSourceAnchor(t *testing.T) {
	Convey("Given unequal candidate strengths", t, func() {
		anchors := CandidateAnchors{
			StrengthBySource: map[SourceType]float64{
				SourceCVD: 0.4,
			},
		}

		weak := EntryCandidate{
			Sources:  []SourceType{SourceCVD},
			Strength: 0.4,
		}
		strong := EntryCandidate{
			Sources:  []SourceType{SourceCVD},
			Strength: 0.8,
		}

		Convey("It should distinguish weak and strong candidates", func() {
			So(normalizedStrength(weak, anchors), ShouldAlmostEqual, 1, 1e-9)
			So(normalizedStrength(strong, anchors), ShouldAlmostEqual, 2, 1e-9)
		})
	})
}
