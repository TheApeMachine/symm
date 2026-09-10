package data_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestReadoutNext(t *testing.T) {
	Convey("Corroboration scales usable authority and discrete values stay raw", t, func() {
		node := data.NewReadout()

		for _, test := range []struct {
			maturity, snr, cred      float64
			supports, contradictions []float64
			defined, discrete        bool
		}{
			{0.8, 5, 1, []float64{0.1}, []float64{0.5}, true, false},
			{0.8, 5, 0.2, []float64{}, []float64{}, true, false},
			{0.8, 5, 1, []float64{}, []float64{}, false, false},
			{0.8, 5, 1, []float64{}, []float64{}, false, true},
		} {
			out, err := transport.Evaluate(node, transport.Values(data.ReadoutInput{
				QualityReading: data.QualityReading{
					Maturity: test.maturity, SNR: test.snr, Estimated: true, SNRDefined: true,
				},
				Raw: 10, Credibility: test.cred, Supports: test.supports,
				Contradictions: test.contradictions, Defined: test.defined, Discrete: test.discrete,
			}))
			So(err, ShouldBeNil)

			authority := test.maturity * test.snr / (1 + test.snr) * test.cred
			support, contradiction := 0.0, 0.0

			for _, value := range test.supports {
				support += value
			}

			for _, value := range test.contradictions {
				contradiction += value
			}

			authority *= (1 + support) / (1 + 2*contradiction)

			if authority > 1 {
				authority = 1
			}

			if !test.defined {
				authority = 0
			}

			So(out.Authority, ShouldAlmostEqual, authority)
			value := 10 * authority

			if test.discrete {
				value = 10
			}

			So(out.Value, ShouldAlmostEqual, value)
		}
	})
}
