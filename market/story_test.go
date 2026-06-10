package market

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

func TestStoryShouldPublishUI(t *testing.T) {
	Convey("Given a story publish interval", t, func() {
		viper.Set("market.story.ui_interval", 500*time.Millisecond)
		start := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)

		Convey("It should publish on the first call", func() {
			story := &Story{}

			So(story.shouldPublishUI(start), ShouldBeTrue)
		})

		Convey("It should suppress publishes inside the interval", func() {
			story := &Story{}

			So(story.shouldPublishUI(start), ShouldBeTrue)
			So(story.shouldPublishUI(start.Add(100*time.Millisecond)), ShouldBeFalse)
		})

		Convey("It should publish again after the interval elapses", func() {
			story := &Story{}

			So(story.shouldPublishUI(start), ShouldBeTrue)
			So(story.shouldPublishUI(start.Add(600*time.Millisecond)), ShouldBeTrue)
		})
	})
}
