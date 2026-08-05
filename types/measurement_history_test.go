package types

import (
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAppendMeasurements(t *testing.T) {
	Convey("Given concurrent signal publications", t, func() {
		thesis := NewThesis()
		var waitGroup sync.WaitGroup

		Convey("When many goroutines publish the same source-symbol row", func() {
			for index := range 128 {
				waitGroup.Go(func() {
					thesis.AppendMeasurements(SourceCVD, &Measurement{
						Source: SourceCVD,
						Symbol: "BTC/USD",
						At:     time.Unix(int64(index), 0).UTC(),
					})
				})
			}

			waitGroup.Wait()
			count := 0
			thesis.Measurements.Range(func(_, value any) bool {
				for _, measurement := range value.([]*Measurement) {
					if measurement != nil {
						count++
					}
				}

				return true
			})

			Convey("Then every cycle row is retained without map corruption", func() {
				So(count, ShouldEqual, 128)
			})
		})
	})
}
