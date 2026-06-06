package perspectives

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestRegimeSizeScale(t *testing.T) {
	Convey("Given configured regime size scales", t, func() {
		testconfig.Load(t)

		scale, err := RegimeSizeScale(types.RegimeTrending)
		So(err, ShouldBeNil)
		So(scale, ShouldAlmostEqual, 1.0, 1e-9)

		scale, err = RegimeSizeScale(types.RegimeChoppy)
		So(err, ShouldBeNil)
		So(scale, ShouldAlmostEqual, 0.5, 1e-9)
	})

	Convey("Given a non-positive regime size scale", t, func() {
		testconfig.Load(t)

		key := "trading.replay.trending_size_scale"
		original := viper.GetFloat64(key)
		viper.Set(key, 0)

		defer viper.Set(key, original)

		_, err := RegimeSizeScale(types.RegimeTrending)
		So(err, ShouldNotBeNil)
	})
}
