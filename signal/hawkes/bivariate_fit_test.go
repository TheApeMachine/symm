package hawkes

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBivariateFitValid(t *testing.T) {
	Convey("Given a well-formed fit", t, func() {
		fit := BivariateFit{
			MuBuy:          1,
			MuSell:         1,
			AlphaBB:        0.1,
			AlphaBS:        0.1,
			AlphaSB:        0.1,
			AlphaSS:        0.1,
			Beta:           1,
			SpectralRadius: 0.5,
		}

		Convey("It should validate parameters", func() {
			So(fit.valid(), ShouldBeTrue)
		})
	})

	Convey("Given an unstable spectral radius", t, func() {
		fit := BivariateFit{MuBuy: 1, MuSell: 1, Beta: 1, SpectralRadius: 1.2}

		Convey("It should reject the fit", func() {
			So(fit.valid(), ShouldBeFalse)
		})
	})
}

func TestBivariateFitWithIntensities(t *testing.T) {
	Convey("Given fitted parameters", t, func() {
		start := time.Now()
		stream := NewArrivalStream(
			[]time.Time{start},
			[]time.Time{start.Add(time.Second)},
		)
		fit := BivariateFit{
			MuBuy: 1, MuSell: 1,
			AlphaBB: 0.2, AlphaBS: 0.1,
			AlphaSB: 0.1, AlphaSS: 0.2,
			Beta: 1,
		}

		Convey("It should compute horizon intensities", func() {
			withIntensities := fit.WithIntensitiesAt(stream, start.Add(2*time.Second))

			So(withIntensities.BuyIntensity, ShouldBeGreaterThan, 0)
			So(withIntensities.SellIntensity, ShouldBeGreaterThan, 0)
		})
	})
}
