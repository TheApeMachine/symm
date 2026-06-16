package logic

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestBuildEigenmodeScores(t *testing.T) {
	convey.Convey("Given measurements across eigenmode families", t, func() {
		measurements := []Measurement{
			{Source: SourceCVD, Confidence: 0.8, Strength: 1.0},
			{Source: SourcePumpDump, Confidence: 0.6, Strength: 1.0},
			{Source: SourceFluid, Confidence: 0.4, Strength: 1.0},
		}

		scores := BuildEigenmodeScores(measurements)
		dominance := 1.4 / 1.8

		convey.Convey("It should boost weaker modes to dominant cluster share", func() {
			convey.So(scores[EigenmodeMomentum], convey.ShouldAlmostEqual, dominance, 1e-12)
			convey.So(scores[EigenmodeStructure], convey.ShouldAlmostEqual, dominance, 1e-12)
		})
	})
}

func TestDominantEnergyEnergyRatio(t *testing.T) {
	convey.Convey("Given local and dominant energies", t, func() {
		ratio := dominantEnergyEnergyRatio(0.4, 1.4, 1.8)

		convey.Convey("It should use geometry-derived dominance", func() {
			convey.So(ratio, convey.ShouldAlmostEqual, 1.4/1.8, 1e-12)
		})
	})
}

func TestEigenmodeScore(t *testing.T) {
	convey.Convey("Given a requested mode with energy", t, func() {
		measurements := []Measurement{
			{Source: SourceCVD, Confidence: 0.5, Strength: 2.0},
		}

		score, ok := EigenmodeScore(measurements, EigenmodeMomentum)

		convey.Convey("It should return the normalized mode score", func() {
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(score, convey.ShouldEqual, 1.0)
		})
	})
}

func BenchmarkBuildEigenmodeScores(b *testing.B) {
	measurements := []Measurement{
		{Source: SourceCVD, Confidence: 0.8, Strength: 1.0},
		{Source: SourcePumpDump, Confidence: 0.6, Strength: 1.0},
		{Source: SourceFluid, Confidence: 0.4, Strength: 1.0},
		{Source: SourceToxicity, Confidence: 0.3, Strength: 1.0},
		{Source: SourceLeadLag, Confidence: 0.2, Strength: 1.0},
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = BuildEigenmodeScores(measurements)
	}
}
