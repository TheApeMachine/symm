package leadlag

import (
	"math"
	"testing"

	"github.com/theapemachine/nomagique/statistic"
)

func TestWindowsFromCountMatchesZeroSeriesResolve(t *testing.T) {
	t.Parallel()

	for _, sampleCount := range []int{1, 4, 9, 16, 25, 100} {
		sampleCount := sampleCount
		short, long, lag := windowsFromCount(sampleCount)
		wantShort, wantLong, err := statistic.ResolveWindows(
			make([]float64, sampleCount), 0, 0,
		)

		if err != nil {
			t.Fatalf("sampleCount=%d ResolveWindows: %v", sampleCount, err)
		}

		wantLag := max(1, int(math.Ceil(math.Sqrt(float64(wantLong)))))

		if wantLong > 1 {
			wantLag = min(wantLag, wantLong-1)
		}

		if short != wantShort || long != wantLong || lag != wantLag {
			t.Fatalf(
				"sampleCount=%d got short/long/lag=%d/%d/%d want %d/%d/%d",
				sampleCount, short, long, lag, wantShort, wantLong, wantLag,
			)
		}
	}
}

func BenchmarkWindowsFromCount(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		windowsFromCount(64)
	}
}
