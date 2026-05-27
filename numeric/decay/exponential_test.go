package decay

import (
	"math"
	"testing"
	"time"

	"github.com/theapemachine/symm/numeric/timeline"
)

func TestIntensityAtAccumulatesDecayedImpulses(t *testing.T) {
	start := time.Unix(0, 0)
	selfEvents := timeline.New([]time.Time{start})
	crossEvents := timeline.Timeline{}
	at := start.Add(2 * time.Second)
	intensity := IntensityAt(selfEvents, crossEvents, at, 1, 2, 0, 1)

	if intensity <= 1 {
		t.Fatalf("expected excitation above baseline, got %v", intensity)
	}
}

func TestKernelSupportIncreasesWithHorizon(t *testing.T) {
	start := time.Unix(0, 0)
	events := timeline.New([]time.Time{start})
	short := KernelSupport(events, start.Add(time.Second), 1)
	long := KernelSupport(events, start.Add(3*time.Second), 1)

	if long <= short {
		t.Fatalf("expected longer horizon support, short=%v long=%v", short, long)
	}
}

func TestLogPositiveGuardsNonPositive(t *testing.T) {
	if math.IsInf(LogPositive(0), 0) {
		t.Fatal("expected finite log for non-positive input")
	}
}
