package manifold

import (
	"context"
	"encoding/binary"
	"io"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/datura/transport"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/signal/compute"
)

const manifoldBatchCapacity = 8192

/*
Manifold is the pilot-wave perspective on systemic field dynamics, classifying
GPU-integrated manifold features per symbol.

1. What it measures exactly (in isolation)

The Manifold signal classifies four features emitted by the Field solver after
ticker ingest and GPU integration:

PressureGradNorm: Cross-axis basis and beta dislocations (shockScore).

CoherenceMag2: Systemic herding / superfluid collapse (herdScore component).

GuidanceSpeed: Pilot-wave trend velocity from aligned Ψ (driftScore component).

ViscosityProxy: Inverse divergence — laminar when large, turbulent when small
(noiseScore component).

Measure seeks precomputed features/{scope} from the tree, then runs
nomagique.Number(equation.NewManifoldstate, probability.NewClassifier).

---

2. Semantically, what story does it tell?

The Manifold signal tells the story of collective field behavior — whether
price action is herd-driven, shock-dislocated, drifting on guidance, or noise.

1. Systemic Herd

Coherence and guidance align into synchronized mass motion.
Indicators: herdScore = coherenceMag2 × guidanceSpeed dominates.
Semantic Meaning: Superfluid collapse — the crowd moves as one.

2. Liquidity Shock

Pressure gradient dislocation exceeds other modes.
Indicators: shockScore = pressureGradNorm dominates.
Semantic Meaning: Field rupture — structural stress at the touch.

3. Pilot-Wave Drift

Guidance outruns viscosity — directed drift without full herd lock-in.
Indicators: driftScore = guidanceSpeed / viscosityProxy dominates.
Semantic Meaning: Synchronized drift on a laminar substrate.

4. Stochastic Noise

Low coherence with residual viscosity — no dominant field mode.
Indicators: noiseScore = viscosityProxy × (1 − coherenceMag2) dominates.
Semantic Meaning: Decoupled noise — no systemic story.

# Summary of Manifold Categories

| Category          | Dominant Score | Primary Input        | Market "Feel"           |
|:------------------|:---------------|:---------------------|:------------------------|
| Systemic Herd     | herdScore      | Coherence × Guidance | Superfluid Collapse     |
| Liquidity Shock   | shockScore     | PressureGradNorm     | Field Rupture           |
| Pilot-Wave Drift  | driftScore     | Guidance / Viscosity | Synchronized Drift      |
| Stochastic Noise  | noiseScore     | Viscosity × (1−Coh.) | Decoupled Noise         |
*/
/*
Signal classifies the 3D manifold state for one symbol from Field features.
See the struct comment block for category semantics.
*/
type Signal struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q[any]
	subscribers *sync.Map
	algo        io.ReadWriteCloser
	tree        *dmt.Tree
	field       *Field
	serial      *compute.SerialPool
}

/*
NewSignal composes the manifold-state pipeline and subscribes to market channels.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	field := errnie.Does(func() (*Field, error) {
		return NewField()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"manifold: failed to create field",
			err,
		))
	}).Value()

	serial := compute.NewSerialPool(
		ctx,
		manifoldBatchCapacity,
		100*time.Millisecond,
	)

	if field != nil {
		field.bindSerial(serial)
	}

	manifoldstate := equation.NewManifoldstate(datura.Acquire("manifold-state", datura.APPJSON))

	return &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		subscribers: &sync.Map{},
		tree:        tree,
		field:       field,
		serial:      serial,
		algo: nomagique.Number(
			manifoldstate,
			probability.NewClassifier(
				datura.Acquire("manifold-classifier", datura.APPJSON).WithAttributes(datura.Map[any]{
					"inputs": []string{"herdScore", "shockScore", "driftScore", "noiseScore"},
				}),
			),
		),
	}
}

func (signal *Signal) IngestRoles() []string {
	return []string{"ticker"}
}

func (signal *Signal) Measure(datapoint *datura.Artifact) *datura.Artifact {
	if signal == nil || datapoint == nil || signal.algo == nil {
		return nil
	}

	scope, _ := datapoint.Scope()

	if scope == "" {
		return nil
	}

	featurePrefix := "features/" + scope
	featurePayload := []byte(nil)

	for inbound := range signal.tree.Seek([]byte(featurePrefix)) {
		if inbound == nil || !inbound.HasEncryptedPayload() {
			continue
		}

		payload := inbound.DecryptPayload()

		if len(payload) == 0 {
			continue
		}

		featurePayload = payload
	}

	featureWire := manifoldFeatureWire(featurePayload)

	if len(featureWire) == 0 {
		return nil
	}

	stored := datura.Acquire("manifold-features", datura.APPJSON)
	stored.WithRole("features")
	stored.WithScope(scope)
	stored.WithPayload(featureWire)

	if transport.NewFlipFlop(
		stored, signal.algo,
	) != nil {
		stored.Release()

		return nil
	}

	stored.WithRole("measurement")
	stored.WithScope(scope)
	stored.Merge("scope", scope)

	category := datura.Peek[float64](stored, "output", "category")
	confidence := datura.Peek[float64](stored, "output", "confidence")
	stored.Merge("classifier.category", int(category))
	stored.Merge("classifier.confidence", confidence)

	if signal.tree != nil {
		if wire := stored.Pack(); len(wire) > 0 {
			signal.tree.Insert(stored.Prefix(), wire)
		}
	}

	uniformConfidence := 1.0 / 4.0

	if confidence <= 0 ||
		math.IsNaN(confidence) ||
		math.IsInf(confidence, 0) ||
		confidence <= uniformConfidence+1e-12 {
		stored.Release()

		return nil
	}

	return stored
}

/*
FieldSnapshot builds the manifold dashboard payload from the last integrated field.
*/
func manifoldFeatureWire(raw []byte) []byte {
	if len(raw) == 0 {
		return nil
	}

	if raw[0] == '{' {
		return raw
	}

	fieldCount := len(raw) / 8
	samples := make([]float64, 0, fieldCount)

	for offset := 0; offset+8 <= len(raw); offset += 8 {
		bits := binary.BigEndian.Uint64(raw[offset : offset+8])
		sample := math.Float64frombits(bits)

		if math.IsNaN(sample) || math.IsInf(sample, 0) {
			continue
		}

		samples = append(samples, sample)
	}

	if len(samples) == 0 {
		return nil
	}

	return equation.MarshalFeatureSchema(equation.ManifoldInputKeys, samples)
}

func (signal *Signal) FieldSnapshot(eventAt time.Time) (map[string]any, error) {
	if signal == nil || signal.field == nil {
		return nil, nil
	}

	if !signal.field.hasPublishableSnapshot() {
		return nil, nil
	}

	return signal.field.snapshotPayload(eventAt)
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	if signal.serial != nil {
		signal.serial.Close()
	}

	if signal.field != nil {
		signal.field.Close()
	}

	return err
}
