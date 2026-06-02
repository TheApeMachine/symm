package hawkes

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestHawkesReading(t *testing.T) {
	Convey("Given a saturated Hawkes fit", t, func() {
		fit := BivariateFit{
			MuBuy:          1,
			MuSell:         1,
			Beta:           1,
			BuyIntensity:   2,
			SellIntensity:  2,
			SpectralRadius: 0.9,
		}

		category, evidence := hawkesReading(fit, 0.1, false)

		Convey("It should classify saturation", func() {
			So(category, ShouldEqual, perspectives.CategorySaturation)
			So(evidence, ShouldBeGreaterThan, 0)
		})
	})
}
