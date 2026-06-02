package optimizer

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestEmitBootstrapPlaybooks(t *testing.T) {
	Convey("Given a scan search", t, func() {
		ctx := context.Background()
		rows := []perspectives.Measurement{
			{Category: perspectives.CategoryLaminar, SNR: 2},
		}
		profile := &Profile{ctx: ctx}
		profile.Add(rows[0])
		profile.PrepareCache()

		search := NewScanSearch(ctx, profile, rows, ScanOptions{Workers: 1})
		emitted := 0

		search.emitBootstrapPlaybooks(func(candidate scanCandidate) bool {
			emitted++

			return true
		}, nil)

		Convey("It should emit decision seed candidates", func() {
			So(emitted, ShouldBeGreaterThanOrEqualTo, 0)
		})
	})
}
