package market

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/internal/testconfig"
)

func TestMeasurementBuffer(t *testing.T) {
	Convey("Given story.measurements.buffer in config", t, func() {
		testconfig.Load(t)

		buffer, err := MeasurementBuffer()
		So(err, ShouldBeNil)
		So(buffer, ShouldEqual, 1024)
	})

	Convey("Given a non-positive story.measurements.buffer", t, func() {
		testconfig.Load(t)

		key := "story.measurements.buffer"
		original := viper.GetInt(key)
		viper.Set(key, 0)

		defer viper.Set(key, original)

		_, err := MeasurementBuffer()
		So(err, ShouldNotBeNil)
	})
}
