package data_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestQualityNext(t *testing.T) {
	Convey("Quality follows support and SNR precedence independently", t, func() {
		quality := data.NewQuality()
		authority := data.NewAuthority()

		for _, facts := range []data.QualityFacts{
			{},
			{Support: 10, Divergence: 4, NoiseVariance: 2, HasSupport: true, HasDivergence: true, HasNoise: true},
			{Support: 10, Divergence: 4, HasSupport: true, HasDivergence: true},
			{Support: 10, Divergence: 4, NoiseVariance: 4, HasSupport: true, HasDivergence: true, HasNoise: true},
			{Support: 20, MahalanobisSNR: 5.5, HasSupport: true, HasMahalanobis: true},
			{Support: 1, MahalanobisSNR: 5.5, HasSupport: true, HasMahalanobis: true},
			{MahalanobisSNR: 5.5, HasMahalanobis: true},
			{Divergence: 2, NoiseVariance: 1, HasDivergence: true, HasNoise: true},
			{Support: 0, HasSupport: true},
			{Support: 4, Divergence: 0, NoiseVariance: 1, HasSupport: true, HasDivergence: true, HasNoise: true},
			{},
		} {
			readingEval := transport.NewEvaluate(quality)
			var reading data.QualityReading

			for out := range readingEval.Next(transport.NewValues(facts).Next(nil)) {
				reading = *(*data.QualityReading)(out)
			}

			err := readingEval.Error()
			So(err, ShouldBeNil)

			snr, defined, maturity := 0.0, false, 1.0

			if facts.HasSupport {
				maturity = 0

				if facts.Support > 1 {
					maturity = 1 - 1/facts.Support

					if facts.HasDivergence && facts.HasNoise && facts.NoiseVariance > 0 {
						candidate := facts.Divergence * facts.Divergence / facts.NoiseVariance

						if !math.IsInf(candidate, 0) && !math.IsNaN(candidate) {
							snr = candidate
							defined = true
						}
					}

					if facts.HasMahalanobis && facts.MahalanobisSNR >= 0 &&
						!math.IsInf(facts.MahalanobisSNR, 0) && !math.IsNaN(facts.MahalanobisSNR) {
						snr = facts.MahalanobisSNR
						defined = true
					}
				}
			}

			estimated := facts.HasSupport || facts.HasDivergence || facts.HasMahalanobis
			So(reading.Maturity, ShouldEqual, maturity)
			So(reading.SNR, ShouldEqual, snr)
			So(reading.SNRDefined, ShouldEqual, defined)
			So(reading.Estimated, ShouldEqual, estimated)

			factor := 1.0

			if estimated {
				factor = 0.5

				if defined {
					factor = 0.1

					if snr > 0 {
						factor = snr / (1 + snr)
					}
				}
			}

			weightEval := transport.NewEvaluate(authority)
			var weight float64

			for out := range weightEval.Next(transport.NewValues(reading).Next(nil)) {
				weight = *(*float64)(out)
			}

			err = weightEval.Error()
			So(err, ShouldBeNil)
			So(weight, ShouldAlmostEqual, math.Min(1, math.Max(0, maturity*factor)))
		}
	})
}

func TestQualityRefusesImmatureNoise(t *testing.T) {
	Convey("Given divergence and tiny noise without mature support", t, func() {
		quality := data.NewQuality()
		facts := data.QualityFacts{
			Divergence:    1,
			NoiseVariance: 1e-30,
			HasDivergence: true,
			HasNoise:      true,
		}
		readingEval := transport.NewEvaluate(quality)
		var reading data.QualityReading
		for out := range readingEval.Next(transport.NewValues(facts).Next(nil)) {
			reading = *(*data.QualityReading)(out)
		}
		So(reading.SNRDefined, ShouldBeFalse)
	})

	Convey("Given Inf Mahalanobis under mature support", t, func() {
		quality := data.NewQuality()
		facts := data.QualityFacts{
			Support:        10,
			HasSupport:     true,
			MahalanobisSNR: math.Inf(1),
			HasMahalanobis: true,
		}
		readingEval := transport.NewEvaluate(quality)
		var reading data.QualityReading
		for out := range readingEval.Next(transport.NewValues(facts).Next(nil)) {
			reading = *(*data.QualityReading)(out)
		}
		So(reading.SNRDefined, ShouldBeFalse)
	})
}


func TestQualityRefusesAstronomicalSNR(t *testing.T) {
	Convey("Given mature support with collapsed noise variance", t, func() {
		quality := data.NewQuality()
		facts := data.QualityFacts{
			Support:        100,
			HasSupport:     true,
			Divergence:     1,
			NoiseVariance:  1e-40,
			HasDivergence:  true,
			HasNoise:       true,
		}
		readingEval := transport.NewEvaluate(quality)
		var reading data.QualityReading
		for out := range readingEval.Next(transport.NewValues(facts).Next(nil)) {
			reading = *(*data.QualityReading)(out)
		}
		So(reading.SNRDefined, ShouldBeFalse)
	})

	Convey("Given Mahalanobis beyond 1/sqrt(eps)", t, func() {
		quality := data.NewQuality()
		facts := data.QualityFacts{
			Support:        100,
			HasSupport:     true,
			MahalanobisSNR: 1e12,
			HasMahalanobis: true,
		}
		readingEval := transport.NewEvaluate(quality)
		var reading data.QualityReading
		for out := range readingEval.Next(transport.NewValues(facts).Next(nil)) {
			reading = *(*data.QualityReading)(out)
		}
		So(reading.SNRDefined, ShouldBeFalse)
	})
}
