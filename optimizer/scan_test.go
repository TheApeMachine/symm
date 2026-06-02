package optimizer

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestNewScanSearch(t *testing.T) {
	Convey("Given replay measurements", t, func() {
		ctx := context.Background()
		rows := []perspectives.Measurement{
			{Symbol: "BTC/EUR", Source: perspectives.SourceFluid, Category: perspectives.CategoryLaminar, SNR: 2, Last: 100},
		}
		profile := &Profile{ctx: ctx}
		profile.Add(rows[0])
		profile.PrepareCache()

		search := NewScanSearch(ctx, profile, rows, ScanOptions{Workers: 1})

		Convey("It should construct a scan search", func() {
			So(search, ShouldNotBeNil)
			So(search.options.Workers, ShouldEqual, 1)
		})
	})
}

func TestRunBeamPhaseNoPanic(t *testing.T) {
	Convey("Given an empty beam phase", t, func() {
		ctx := context.Background()
		profile := &Profile{ctx: ctx}
		search := NewScanSearch(ctx, profile, nil, ScanOptions{Workers: 1})

		Convey("It should finish scoring with zero candidates", func() {
			So(func() {
				search.runBeamPhase("test", func(send func(scanCandidate) bool) {})
			}, ShouldNotPanic)
		})
	})
}
