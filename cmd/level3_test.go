package cmd

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	kraken "github.com/theapemachine/symm/kraken/market"
)

func TestConfigureLevel3WithoutCredentials(t *testing.T) {
	Convey("Given empty API credentials", t, func() {
		err := configureLevel3(context.Background(), "", "")

		Convey("It should leave level3 unavailable", func() {
			So(err, ShouldBeNil)
			So(kraken.Level3Available(), ShouldBeFalse)
		})
	})
}
