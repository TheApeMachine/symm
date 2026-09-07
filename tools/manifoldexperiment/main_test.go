package main

import (
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
	"strings"
	"testing"
)

func TestExperimentCommandRun(t *testing.T) {
	if err := (&ExperimentCommand{}).Run(); err == nil {
		t.Fatal("implicit compute budget accepted")
	}
}
func TestReplaySchemaAndShape(t *testing.T) {
	command := &ExperimentCommand{Steps: 1, Grid: 2}
	// These fail before any Metal initialization, not through a fake GPU runner.
	for _, input := range []string{`{"symbol":"BTC/USD"}`, `{"schema":"old-market-format"}`, `{"schema":"sensorium-state-replay/v1","state":{"N":1}}`} {
		if _, err := command.replay(strings.NewReader(input)); err == nil {
			t.Fatal("invalid replay input accepted")
		}
	}
	frame := ReplayFrame{Schema: replaySchema, State: &sensorium.State{N: 1}}
	if frame.validate() == nil {
		t.Fatal("missing particle tensors accepted")
	}
}
