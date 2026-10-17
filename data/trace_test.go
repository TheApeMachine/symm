package data

import (
	"testing"
)

func TestTraceStaysOffTheWire(t *testing.T) {
	value := NewValue("workspace", "tick", "aggregate")
	before := value.Size()

	value.Note(Span{Name: "cvd", Group: "signal", WorkNs: 1500, WaitNs: 40})
	value.MarkEnqueued(10)
	value.MarkStageDone(0, 20)

	if value.Size() != before {
		t.Fatalf("noting a span changed the stored size from %d to %d", before, value.Size())
	}

	decoded := From(value.Bytes())

	if decoded == nil {
		t.Fatal("round trip returned nil")
	}

	if spans := decoded.Spans(); len(spans) != 0 {
		t.Fatalf("decoded value carried %d spans", len(spans))
	}

	if got := value.Spans(); len(got) != 1 || got[0].Name != "cvd" || got[0].WorkNs != 1500 {
		t.Fatalf("span = %+v", got)
	}

	if value.Enqueued() != 10 || value.StageDone(0) != 20 {
		t.Fatalf("clock enqueued %d stage %d", value.Enqueued(), value.StageDone(0))
	}
}
