package cmd

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/optimizer"
)

func TestValidTuneScanOptions(t *testing.T) {
	convey.Convey("Given valid scan options", t, func() {
		options, err := validTuneScanOptions(optimizer.ScanOptions{
			Workers:        2,
			MaxThresholds:  0,
			BeamWidth:      16,
			CandidateLimit: 0,
		})

		convey.Convey("It should accept the options", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(options.Workers, convey.ShouldEqual, 2)
		})
	})

	convey.Convey("Given invalid scan options", t, func() {
		_, err := validTuneScanOptions(optimizer.ScanOptions{
			Workers:        0,
			MaxThresholds:  0,
			BeamWidth:      16,
			CandidateLimit: 0,
		})

		convey.Convey("It should reject the options", func() {
			convey.So(err, convey.ShouldNotBeNil)
		})
	})
}
