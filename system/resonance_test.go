package system

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewResonance(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewResonance is called", func() {
			res := NewResonance()

			Convey("solver defaults land from viper", func() {
				So(res.LearningRate, ShouldEqual, 0.01)
				So(res.Layers, ShouldEqual, 3)
			})
		})
	})
}
