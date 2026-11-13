package cmd

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
)

func TestNewBooter(t *testing.T) {
	Convey("Setup", t, func() {
		system.Cfg = system.NewConfig()
		system.Cfg.Market.Model = "paper"

		Convey("When NewBooter is called in paper mode", func() {
			booter := NewBooter(context.Background(), "symm-test")

			Convey("It names the system and stays non-live", func() {
				So(booter, ShouldNotBeNil)
				So(booter.Name(), ShouldEqual, "symm-test")
				So(booter.live, ShouldBeFalse)
				So(booter.credErr, ShouldBeNil)
			})
		})
	})
}
