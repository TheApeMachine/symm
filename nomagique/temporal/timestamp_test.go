package temporal_test

import (
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/tests"
	"testing"
	"time"
)

func TestTimestampNext(t *testing.T) {
	stamp := time.Unix(1700000000, 123)
	node := temporal.NewTimestamp()
	outputs := tests.Drain(t, node, tests.Values(stamp))
	tests.Sound(t, node)
	if len(outputs) != 1 || outputs[0] != stamp.UnixNano() {
		t.Fatal(outputs)
	}
}
