package timeline

import (
	"testing"
	"time"
)

func TestTimelineGaps(t *testing.T) {
	start := time.Unix(0, 0)
	timeline := New([]time.Time{
		start,
		start.Add(2 * time.Second),
		start.Add(2 * time.Second),
		start.Add(5 * time.Second),
	})
	gaps := timeline.Gaps()

	if len(gaps) != 2 {
		t.Fatalf("expected two positive gaps, got %d", len(gaps))
	}

	if gaps[0] != 2 {
		t.Fatalf("expected first gap 2s, got %v", gaps[0])
	}

	if gaps[1] != 3 {
		t.Fatalf("expected second gap 3s, got %v", gaps[1])
	}
}

func TestTimelineSpan(t *testing.T) {
	start := time.Unix(0, 0)
	timeline := New([]time.Time{start, start.Add(time.Second)})
	span := timeline.Span(start.Add(4 * time.Second))

	if span != 4 {
		t.Fatalf("expected span 4s, got %v", span)
	}
}

func TestTimelineNewSortsUnsortedInput(t *testing.T) {
	start := time.Unix(0, 0)
	timeline := New([]time.Time{
		start.Add(3 * time.Second),
		start,
		start.Add(time.Second),
	})

	if !timeline.Times()[0].Equal(start) {
		t.Fatalf("expected sorted timeline, got %v", timeline.Times())
	}
}
