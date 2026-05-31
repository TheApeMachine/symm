package trader

import (
	"fmt"
	"sync"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestRefreshCrossSectionConcurrentRecord(t *testing.T) {
	convey.Convey("Given concurrent measurements and cross-section refresh", t, func() {
		crypto := newTestCrypto()
		var waitGroup sync.WaitGroup

		for worker := 0; worker < 8; worker++ {
			waitGroup.Go(func() {
				for index := 0; index < 200; index++ {
					symbol := fmt.Sprintf("COIN%d/EUR", index%32)
					crypto.record(traderMeasurement(
						symbol,
						perspectives.SourceCVD,
						perspectives.CategoryAggressiveDrive,
						float64(index%5)+1,
					))
				}
			})
		}

		for worker := 0; worker < 4; worker++ {
			waitGroup.Go(func() {
				for index := 0; index < 50; index++ {
					crypto.refreshCrossSection()
				}
			})
		}

		waitGroup.Wait()

		convey.Convey("It should rebuild cross-section without panicking", func() {
			snapshot := crypto.crossSection.Load()
			convey.So(snapshot, convey.ShouldNotBeNil)
		})
	})
}
