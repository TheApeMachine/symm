package optimizer

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestRankedEntryBranchers(t *testing.T) {
	convey.Convey("Given more branchers than beam width", t, func() {
		ctx := context.Background()
		rows := []perspectives.Measurement{
			{Symbol: "BTC/EUR", Source: perspectives.SourceFluid, Category: perspectives.CategoryLaminar, SNR: 2, Last: 100},
			{Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion, Category: perspectives.CategoryExhaustion, SNR: 3, Last: 110},
		}
		profile := &Profile{ctx: ctx}

		for _, row := range rows {
			profile.Add(row)
		}

		profile.PrepareCache()
		search := NewScanSearch(ctx, profile, rows, ScanOptions{Workers: 1, BeamWidth: 1})
		branchers := search.rankedEntryBranchers()

		convey.Convey("It should rank and cap entry branchers to beam width", func() {
			convey.So(len(branchers), convey.ShouldEqual, 1)
		})
	})
}

func TestScanSearchBranchers(t *testing.T) {
	convey.Convey("Given replay measurements", t, func() {
		ctx := context.Background()
		rows := []perspectives.Measurement{
			{Symbol: "BTC/EUR", Source: perspectives.SourceFluid, Category: perspectives.CategoryLaminar, SNR: 2, Last: 100},
		}
		profile := &Profile{ctx: ctx}
		profile.Add(rows[0])
		profile.PrepareCache()

		search := NewScanSearch(ctx, profile, rows, ScanOptions{Workers: 1})
		branchers := search.branchers()

		convey.Convey("It should expose candidate entry branchers", func() {
			convey.So(len(branchers), convey.ShouldBeGreaterThan, 0)
		})
	})
}
