package learning_test

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/tests"
	"testing"
	"time"
)

func TestRewardNext(t *testing.T) {
	node := learning.NewReward()
	start := time.Unix(1700000000, 0).UnixNano()
	marks := []core.Primitive{
		tests.Record(map[string]any{"at": start, "version": uint64(1), "value": 100.0}),
		tests.Record(map[string]any{"at": start + int64(1e9), "version": uint64(2), "value": 110.0}),
		tests.Record(map[string]any{"at": start + int64(3e9), "version": uint64(3), "value": 90.0}),
		tests.Record(map[string]any{"at": start + int64(3e9), "version": uint64(3), "value": 90.0}),
		tests.Record(map[string]any{"at": start + int64(3e9), "version": uint64(4), "value": 95.0}),
		tests.Record(map[string]any{"at": start + int64(10e9), "version": uint64(5), "value": 130.0}),
	}
	output := tests.Drain(t, node, tests.Values(marks...))
	tests.Sound(t, node)
	if len(output) != len(marks) {
		t.Fatal(output)
	}
	for index, want := range []float64{0, 10, -20, -20, 5, 35} {
		tests.EqualNumber(t, tests.Number(t, tests.Fields(t, output[index]), "reward"), want)
	}
	tests.EqualNumber(t, tests.Number(t, tests.Fields(t, output[2]), "differential"), -40)
	tests.EqualNumber(t, tests.Number(t, tests.Fields(t, output[4]), "differential"), 5)
	tests.EqualNumber(t, tests.Number(t, tests.Fields(t, output[5]), "rate"), 3)
	tests.EqualNumber(t, tests.Number(t, tests.Fields(t, output[5]), "total_reward"), 30)
	if core.To[uint64](tests.Fields(t, output[5])["transitions"]) != 4 {
		t.Fatal("duplicate changed transitions")
	}
	coarse := learning.NewReward()
	coarseOut := tests.Drain(t, coarse, tests.Values(marks[0], marks[5]))
	tests.EqualNumber(t, tests.Number(t, tests.Fields(t, coarseOut[1]), "rate"), 3)
	// A bad version must be observable and must not be presented as a new outcome.
	bad := tests.Drain(t, node, tests.Values(marks[1]))
	if node.Error() == nil || len(bad) != 0 {
		t.Fatal("accepted regressed mark", bad)
	}
}
