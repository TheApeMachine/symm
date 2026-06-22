package hawkes

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
)

func TestSearchFrenzyFixture(t *testing.T) {
	warmCounts := []int{64, 96, 128, 160}
	warmIntervals := []time.Duration{50 * time.Millisecond, 100 * time.Millisecond, 200 * time.Millisecond}
	bursts := []int{20, 32, 48, 64}
	burstIntervals := []time.Duration{80 * time.Millisecond, 120 * time.Millisecond, 200 * time.Millisecond, 350 * time.Millisecond}
	qtys := []float64{2, 4, 6, 10}

	for _, warmCount := range warmCounts {
		for _, warmInterval := range warmIntervals {
			for _, burst := range bursts {
				for _, burstInterval := range burstIntervals {
					for _, qty := range qtys {
						signal := NewSignal(context.Background(), newTestPool(t), dmt.NewTree(""))
						base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
						for index := range warmCount {
							side := "sell"
							if index%2 == 1 {
								side = "buy"
							}
							at := base.Add(time.Duration(index) * warmInterval).UnixNano()
							frame := tradeDatapoint("ALT/EUR", side, 1, 1, at)
							_ = signal.Measure(frame)
							frame.Release()
						}
						burstStart := base.Add(time.Duration(warmCount) * warmInterval)
						var result *datura.Artifact
						for index := range burst {
							at := burstStart.Add(time.Duration(index) * burstInterval).UnixNano()
							book := bookDatapoint(40, 6, at)
							_ = signal.Measure(book)
							book.Release()
							frame := tradeDatapoint("ALT/EUR", "buy", 1+float64(index)*0.001, qty, at)
							for range 4 {
								measured := signal.Measure(frame)
								if measured != nil {
									if result != nil {
										result.Release()
									}
									result = measured
								}
							}
							frame.Release()
						}
						if result == nil {
							_ = signal.Close()
							continue
						}
						frenzy := outputScore(result, "frenzy")
						cat := categoryResult(result)
						if cat == 1 || frenzy > 0.01 {
							fmt.Printf("HIT warm=%d/%s burst=%d/%s qty=%g cat=%d f=%g s=%g o=%g c=%g\n",
								warmCount, warmInterval, burst, burstInterval, qty, cat,
								frenzy, outputScore(result, "saturation"), outputScore(result, "organic"), outputScore(result, "confidence"))
						}
						if cat == 1 && frenzy > outputScore(result, "organic") && frenzy > outputScore(result, "saturation") {
							fmt.Printf("PASS_CANDIDATE warm=%d/%s burst=%d/%s qty=%g\n", warmCount, warmInterval, burst, burstInterval, qty)
						}
						result.Release()
						_ = signal.Close()
					}
				}
			}
		}
	}
}
