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
			reading, err := transport.Evaluate(quality, transport.Values(facts))
			So(err, ShouldBeNil)

			snr, defined, maturity := 0.0, false, 1.0

			if facts.HasDivergence && facts.HasNoise && facts.NoiseVariance > 0 {
				snr = facts.Divergence * facts.Divergence / facts.NoiseVariance
				defined = true
			}

			if facts.HasSupport {
				maturity = 0

				if facts.Support > 1 {
					maturity = 1 - 1/facts.Support

					if facts.HasMahalanobis && facts.MahalanobisSNR >= 0 {
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

			weight, err := transport.Evaluate(authority, transport.Values(reading))
			So(err, ShouldBeNil)
			So(weight, ShouldAlmostEqual, math.Min(1, math.Max(0, maturity*factor)))
		}
	})
}
