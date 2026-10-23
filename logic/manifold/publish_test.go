package manifold

import (
	"testing"
	"time"

	"github.com/theapemachine/symm/nomagique/physics/sensorium"
	"github.com/theapemachine/symm/types"
)

func TestPublishReadingScalarLayers(t *testing.T) {
	state := &State{
		At:      time.Now(),
		Version: 1,
		GridX:   4,
		GridY:   4,
		GridZ:   4,
		State: &sensorium.State{
			N: 1,
		},
		Reading: sensorium.Reading{
			Divergence:    0.1,
			GuidanceSpeed: 1.2,
			KuramotoR:     0.5,
		},
		Resultants: []types.PhaseChannelResultant{
			{Side: "bid", Count: 1, TotalAmplitude: 1, Coherence: 0.9, Phase: 0.1},
		},
	}

	state.State.Pos = []float32{0.1, 0.2, 0.3}
	state.State.Vel = []float32{0, 0, 0}
	state.State.Mass = []float32{1}
	state.State.Heat = []float32{0.5}
	state.State.Energy = []float32{1}
	state.State.Phase = []float32{0}
	state.State.Omega = []float32{1}
	state.State.Amp = []float32{1}
	state.State.TokenIDs = []int64{0}

	value := PublishReading(state, "BTC/USD", 1, 2, 3)
	if value == nil {
		t.Fatal("expected published value")
	}

	metrics := map[string]float64{}
	for _, layer := range value.LookupPrefix("") {
		var readings []float64
		if err := layer.Decode(&readings); err != nil || len(readings) == 0 {
			continue
		}
		metrics[layer.Key] = readings[0]
	}

	if metrics["divergence"] != 0.1 {
		t.Fatalf("divergence %v", metrics["divergence"])
	}
	if metrics["particle_count"] != 1 {
		t.Fatalf("particle_count %v", metrics["particle_count"])
	}
	if metrics["resultant:bid:coherence"] != 0.9 {
		t.Fatalf("bid coherence %v", metrics["resultant:bid:coherence"])
	}
}
