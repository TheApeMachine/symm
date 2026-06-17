package fluid

import (
	"context"
	"io"
	"math"

	"github.com/theapemachine/datura"
	feed "github.com/theapemachine/symm/signal"
)

type Features struct {
	ctx      context.Context
	cancel   context.CancelFunc
	scope    string
	registry *Registry
}

func NewFeatures(ctx context.Context, registry *Registry) *Features {
	ctx, cancel := context.WithCancel(ctx)

	return &Features{
		ctx:      ctx,
		cancel:   cancel,
		registry: registry,
	}
}

func (features *Features) Artifact() *datura.Artifact {
	state := features.registry.loadSymbol(features.scope)

	if state == nil {
		return nil
	}

	reading, ok := state.Reading()

	if !ok {
		return nil
	}

	turbulentFloor, turbulentReady := reading.dynamics.turbulentReynoldsFloor()
	icebergScore := reading.dynamics.icebergScore(reading.midAddRate, reading.midExecuteRate)

	turbulentReadyFlag := 0.0

	if turbulentReady {
		turbulentReadyFlag = 1
	}

	changePct := state.changePct

	if changePct <= 0 && reading.spreadBPS > 0 {
		changePct = reading.spreadBPS / 10000
	}

	artifact := datura.Acquire("fluid-features", datura.Artifact_Type_json)
	artifact.WithRole("features")
	artifact.WithScope(features.scope)
	artifact.WithPayload(feed.EncodePayload(
		reading.reynolds,
		math.Abs(reading.divergence),
		reading.viscosity,
		reading.midAddRate,
		reading.midExecuteRate,
		reading.dynamics.laminarReynoldsCeiling(reading.reynolds),
		turbulentFloor,
		turbulentReadyFlag,
		reading.dynamics.laminarDivergenceEdge(),
		icebergScore,
		reading.price,
		reading.spreadBPS,
		changePct,
		state.volume,
	))

	return artifact
}

func (features *Features) Read(p []byte) (int, error) {
	artifact := features.Artifact()

	if artifact == nil {
		return 0, io.EOF
	}

	return feed.ReadFeatureArtifact(p, artifact)
}
