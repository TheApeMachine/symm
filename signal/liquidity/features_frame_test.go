package liquidity

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	feed "github.com/theapemachine/symm/signal"
)

func TestFeaturesReadFitsFeatureFrame(testingTB *testing.T) {
	Convey("Given a cross-section with more peers than the frame allows", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		base := time.Date(2026, 6, 17, 2, 0, 0, 0, time.UTC)

		for index := range 1100 {
			symbol := fmt.Sprintf("SYM%d/USD", index)

			if index == 0 {
				symbol = "ETH/USD"
			}

			observeRow(
				signal,
				symbol,
				100+float64(index),
				1,
				float64(index+1),
				0,
				base.Add(time.Duration(index)*time.Millisecond),
			)
		}

		seedTickers(signal, "ETH/USD", base, 8, 100, 500)
		signal.features.scope = "ETH/USD"

		frame := make([]byte, feed.FeatureFrameSize)

		Convey("When Features.Read is called", func() {
			readCount, readErr := signal.features.Read(frame)

			Convey("It should fit the feature frame without short buffer", func() {
				So(readErr, ShouldNotEqual, io.ErrShortBuffer)
				So(readCount, ShouldBeGreaterThan, 0)
				So(readCount, ShouldBeLessThanOrEqualTo, len(frame))
			})
		})
	})
}
